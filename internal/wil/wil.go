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
	"sync"

	"github.com/pyq0109/mirgo/internal/log"
)

func widthBytes(w, bpp int) int {
	return (((w * bpp) + 31) / 32) * 4
}

type Image struct {
	Width  int
	Height int
	HotX   int16
	HotY   int16
	RGBA   *image.RGBA
}

type File struct {
	Title      string
	Count      int
	Images     []*Image
	Palette    [256]color.RGBA
	btVersion  int
	colorCount int

	mu      sync.Mutex
	file    *os.File
	offsets []int32
	path    string
}

// resolveInsensitive 在 target 所在目录中查找基名（不含扩展名）大小写不敏感匹配、
// 且扩展名为 ext（不敏感）的文件，返回完整路径；未找到返回 ""。
func resolveInsensitive(target, ext string) string {
	dir := filepath.Dir(target)
	base := strings.ToLower(strings.TrimSuffix(filepath.Base(target), filepath.Ext(target)))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.EqualFold(filepath.Ext(name), ext) {
			nameBase := strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))
			if nameBase == base {
				return filepath.Join(dir, name)
			}
		}
	}
	return ""
}

func Load(wilPath string) (*File, error) {
	f, err := os.Open(wilPath)
	if err != nil {
		if resolved := resolveInsensitive(wilPath, ".wil"); resolved != "" {
			f, err = os.Open(resolved)
			if err != nil {
				return nil, fmt.Errorf("open %s: %w", wilPath, err)
			}
			wilPath = resolved
		} else {
			return nil, fmt.Errorf("open %s: %w", wilPath, err)
		}
	}

	magic := make([]byte, 5)
	if _, err := f.Read(magic); err != nil {
		f.Close()
		return nil, fmt.Errorf("read magic: %w", err)
	}

	wf := &File{file: f, path: wilPath}
	isILib := string(magic) == "#ILIB"

	if isILib {
		if _, err := f.Seek(40, io.SeekStart); err != nil {
			f.Close()
			return nil, err
		}
		var verFlag int32
		binary.Read(f, binary.LittleEndian, &verFlag)
		if verFlag == 0 {
			wf.btVersion = 0
		} else {
			wf.btVersion = 1
		}
		var imgCount, colorCount, paletteSize int32
		binary.Read(f, binary.LittleEndian, &imgCount)
		binary.Read(f, binary.LittleEndian, &colorCount)
		binary.Read(f, binary.LittleEndian, &paletteSize)
		wf.Count = int(imgCount)
		wf.colorCount = int(colorCount)
		wf.Title = "#ILIB"
	} else {
		title := make([]byte, 36)
		if _, err := f.Read(title); err != nil {
			f.Close()
			return nil, err
		}
		wf.Title = strings.TrimRight(string(magic)+string(title), "\x00")

		var imgCount, colorCount, paletteSize, verFlag int32
		binary.Read(f, binary.LittleEndian, &imgCount)
		binary.Read(f, binary.LittleEndian, &colorCount)
		binary.Read(f, binary.LittleEndian, &paletteSize)
		binary.Read(f, binary.LittleEndian, &verFlag)
		wf.Count = int(imgCount)
		wf.colorCount = int(colorCount)
		if verFlag == 0 {
			wf.btVersion = 1
			f.Seek(-4, io.SeekCurrent)
		} else {
			wf.btVersion = 0
		}
	}

	if wf.Count <= 0 || wf.Count > 100000 {
		f.Close()
		return nil, fmt.Errorf("invalid image count: %d", wf.Count)
	}

	palData := make([]byte, 256*4)
	if _, err := f.Read(palData); err != nil {
		f.Close()
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
	wf.Palette[0].A = 0
	log.Logf(log.LevelInfo, "WIL", "%s palette[0] RGB=(%d,%d,%d)", filepath.Base(wilPath), wf.Palette[0].R, wf.Palette[0].G, wf.Palette[0].B)

	wixPath := strings.TrimSuffix(wilPath, filepath.Ext(wilPath)) + ".wix"
	if _, err := os.Stat(wixPath); err != nil {
		dir := filepath.Dir(wilPath)
		wilBase := strings.ToLower(strings.TrimSuffix(filepath.Base(wilPath), filepath.Ext(wilPath)))
		var matches []string
		for _, pat := range []string{"*.wix", "*.WIX"} {
			if m, _ := filepath.Glob(filepath.Join(dir, pat)); m != nil {
				matches = append(matches, m...)
			}
		}
		for _, m := range matches {
			mBase := strings.TrimRight(strings.ToLower(strings.TrimSuffix(filepath.Base(m), filepath.Ext(m))), ".")
			if mBase == wilBase {
				wixPath = m
				break
			}
		}
	}
	offsets, err := loadWix(wixPath, wf.Count, wf.btVersion)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("load wix: %w", err)
	}

	wf.offsets = offsets
	wf.Images = make([]*Image, wf.Count)

	log.Logf(log.LevelDebug, "WIL", "已加载 %s：%d 张图片（懒加载）", filepath.Base(wilPath), wf.Count)
	return wf, nil
}

func (wf *File) BtVersion() int  { return wf.btVersion }
func (wf *File) ColorCount() int { return wf.colorCount }

func (wf *File) GetImage(idx int) *Image {
	if idx < 0 || idx >= wf.Count {
		return nil
	}

	wf.mu.Lock()
	defer wf.mu.Unlock()

	if wf.Images[idx] != nil {
		return wf.Images[idx]
	}

	img := wf.decodeImage(idx)
	wf.Images[idx] = img
	return img
}

func (wf *File) decodeImage(idx int) *Image {
	if wf.file == nil {
		log.Logf(log.LevelWarn, "WIL", "image %d: file closed", idx)
		return &Image{}
	}

	if _, err := wf.file.Seek(int64(wf.offsets[idx]), io.SeekStart); err != nil {
		log.Logf(log.LevelWarn, "WIL", "image %d: seek offset %d: %v", idx, wf.offsets[idx], err)
		return &Image{}
	}

	var info struct {
		Width  int16
		Height int16
		HotX   int16
		HotY   int16
	}
	if err := binary.Read(wf.file, binary.LittleEndian, &info); err != nil {
		log.Logf(log.LevelWarn, "WIL", "image %d: read header at offset %d: %v", idx, wf.offsets[idx], err)
		return &Image{}
	}

	if wf.btVersion == 0 {
		var bits [4]byte
		binary.Read(wf.file, binary.LittleEndian, &bits)
	}

	w, h := int(info.Width), int(info.Height)
	if w <= 0 || h <= 0 || w > 4096 || h > 4096 {
		log.Logf(log.LevelWarn, "WIL", "image %d: bad dimensions %dx%d", idx, w, h)
		return &Image{Width: w, Height: h, HotX: info.HotX, HotY: info.HotY}
	}

	rgba := image.NewRGBA(image.Rect(0, 0, w, h))

	if wf.colorCount > 256 {
		rowBytes := widthBytes(w, 16)
		raw := make([]byte, rowBytes*h)
		if _, err := io.ReadFull(wf.file, raw); err != nil {
			log.Logf(log.LevelWarn, "WIL", "image %d: read %d bytes failed: %v", idx, len(raw), err)
			return &Image{}
		}
		for y := 0; y < h; y++ {
			rowOff := y * rowBytes
			for x := 0; x < w; x++ {
				v := uint16(raw[rowOff+x*2]) | uint16(raw[rowOff+x*2+1])<<8
				off := (y*w + x) * 4
				if v == 0 {
					rgba.Pix[off+3] = 0
				} else {
					rgba.Pix[off+0] = byte((v >> 11) << 3)
					rgba.Pix[off+1] = byte(((v >> 5) & 0x3F) << 2)
					rgba.Pix[off+2] = byte((v & 0x1F) << 3)
					rgba.Pix[off+3] = 255
				}
			}
		}
	} else {
		rowBytes := widthBytes(w, 8)
		raw := make([]byte, rowBytes*h)
		if _, err := io.ReadFull(wf.file, raw); err != nil {
			log.Logf(log.LevelWarn, "WIL", "image %d: read %d bytes failed: %v", idx, len(raw), err)
			return &Image{}
		}
		for y := 0; y < h; y++ {
			rowOff := y * rowBytes
			for x := 0; x < w; x++ {
				pidx := raw[rowOff+x]
				off := (y*w + x) * 4
				c := wf.Palette[pidx]
				rgba.Pix[off+0] = c.R
				rgba.Pix[off+1] = c.G
				rgba.Pix[off+2] = c.B
				rgba.Pix[off+3] = c.A
			}
		}
	}

	stride := w * 4
	tmp := make([]byte, stride)
	for r := 0; r < h/2; r++ {
		top := rgba.Pix[r*stride : (r+1)*stride]
		bot := rgba.Pix[(h-1-r)*stride : (h-r)*stride]
		copy(tmp, top)
		copy(top, bot)
		copy(bot, tmp)
	}

	return &Image{
		Width:  w,
		Height: h,
		HotX:   info.HotX,
		HotY:   info.HotY,
		RGBA:   rgba,
	}
}

func (wf *File) Close() {
	wf.mu.Lock()
	defer wf.mu.Unlock()
	if wf.file != nil {
		wf.file.Close()
		wf.file = nil
	}
}

func loadWix(wixPath string, expectedCount int, btVersion int) ([]int32, error) {
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
		title := make([]byte, 36)
		f.Read(title)
	}

	var indexCount int32
	binary.Read(f, binary.LittleEndian, &indexCount)
	if !isILib {
		var verFlag int32
		binary.Read(f, binary.LittleEndian, &verFlag)
		if btVersion == 1 {
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
