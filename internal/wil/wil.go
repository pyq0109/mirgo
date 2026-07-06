package wil

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Image holds a single decoded WIL image with RGBA pixels.
type Image struct {
	Width  int
	Height int
	HotX   int16
	HotY   int16
	RGBA   *image.RGBA
}

// File holds a loaded WIL file with decoded images.
type File struct {
	Title    string
	Count    int
	Images   []*Image
	Palette  [256]color.RGBA
}

// Load reads a WIL file and its companion WIX index.
func Load(wilPath string) (*File, error) {
	f, err := os.Open(wilPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", wilPath, err)
	}
	defer f.Close()

	// Detect ILib format
	magic := make([]byte, 5)
	if _, err := f.Read(magic); err != nil {
		return nil, fmt.Errorf("read magic: %w", err)
	}

	wf := &File{}
	isILib := string(magic) == "#ILIB"

	if isILib {
		// ILib: skip to offset 44
		if _, err := f.Seek(44, io.SeekStart); err != nil {
			return nil, err
		}
		var imgCount, colorCount, paletteSize int32
		binary.Read(f, binary.LittleEndian, &imgCount)
		binary.Read(f, binary.LittleEndian, &colorCount)
		binary.Read(f, binary.LittleEndian, &paletteSize)
		wf.Count = int(imgCount)
		wf.Title = "#ILIB"
	} else {
		// Standard: 40-byte title + fields
		title := make([]byte, 35)
		if _, err := f.Read(title); err != nil {
			return nil, err
		}
		wf.Title = strings.TrimRight(string(magic)+string(title), "\x00")

		var imgCount, colorCount, paletteSize, verFlag int32
		binary.Read(f, binary.LittleEndian, &imgCount)
		binary.Read(f, binary.LittleEndian, &colorCount)
		binary.Read(f, binary.LittleEndian, &paletteSize)
		binary.Read(f, binary.LittleEndian, &verFlag)
		wf.Count = int(imgCount)
		if verFlag == 0 {
			f.Seek(-4, io.SeekCurrent)
		}
	}

	if wf.Count <= 0 || wf.Count > 100000 {
		return nil, fmt.Errorf("invalid image count: %d", wf.Count)
	}

	// Read 256-color palette (BGRA)
	palData := make([]byte, 256*4)
	if _, err := f.Read(palData); err != nil {
		return nil, fmt.Errorf("read palette: %w", err)
	}
	for i := 0; i < 256; i++ {
		off := i * 4
		wf.Palette[i] = color.RGBA{
			R: palData[off+2],
			G: palData[off+1],
			B: palData[off+0],
			A: 255,
		}
	}
	wf.Palette[0].A = 0 // index 0 = transparent

	// Load WIX index
	wixPath := strings.TrimSuffix(wilPath, filepath.Ext(wilPath)) + ".wix"
	offsets, err := loadWix(wixPath, wf.Count)
	if err != nil {
		return nil, fmt.Errorf("load wix: %w", err)
	}

	// Read images
	wf.Images = make([]*Image, wf.Count)
	for i := 0; i < wf.Count; i++ {
		if _, err := f.Seek(int64(offsets[i]), io.SeekStart); err != nil {
			wf.Images[i] = &Image{} // empty
			continue
		}

		var info struct {
			Width  int16
			Height int16
			HotX   int16
			HotY   int16
		}
		if err := binary.Read(f, binary.LittleEndian, &info); err != nil {
			wf.Images[i] = &Image{}
			continue
		}

		w, h := int(info.Width), int(info.Height)
		if w <= 0 || h <= 0 || w > 4096 || h > 4096 {
			wf.Images[i] = &Image{Width: w, Height: h, HotX: info.HotX, HotY: info.HotY}
			continue
		}

		pixels := make([]byte, w*h)
		if _, err := f.Read(pixels); err != nil {
			wf.Images[i] = &Image{}
			continue
		}

		// Convert palette indices to RGBA
		rgba := image.NewRGBA(image.Rect(0, 0, w, h))
		for j, idx := range pixels {
			off := j * 4
			c := wf.Palette[idx]
			rgba.Pix[off+0] = c.R
			rgba.Pix[off+1] = c.G
			rgba.Pix[off+2] = c.B
			rgba.Pix[off+3] = c.A
		}

		wf.Images[i] = &Image{
			Width:  w,
			Height: h,
			HotX:   info.HotX,
			HotY:   info.HotY,
			RGBA:   rgba,
		}
	}

	return wf, nil
}

func loadWix(wixPath string, expectedCount int) ([]int32, error) {
	f, err := os.Open(wixPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", wixPath, err)
	}
	defer f.Close()

	magic := make([]byte, 5)
	if _, err := f.Read(magic); err != nil {
		return nil, err
	}

	isILib := string(magic) == "#INDX"
	if isILib {
		f.Seek(44, io.SeekStart)
	} else {
		title := make([]byte, 35)
		f.Read(title)
	}

	var indexCount int32
	binary.Read(f, binary.LittleEndian, &indexCount)
	if !isILib {
		var verFlag int32
		binary.Read(f, binary.LittleEndian, &verFlag)
		if verFlag == 0 {
			f.Seek(-4, io.SeekCurrent)
		}
	}

	if int(indexCount) != expectedCount {
		return nil, fmt.Errorf("index count mismatch: wix=%d, wil=%d", indexCount, expectedCount)
	}

	offsets := make([]int32, indexCount)
	if err := binary.Read(f, binary.LittleEndian, offsets); err != nil {
		return nil, fmt.Errorf("read offsets: %w", err)
	}

	return offsets, nil
}
