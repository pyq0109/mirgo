package mapformat

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

const (
	headerSize = 52
	cellSize   = 12
)

// Header is the 52-byte map file header.
type Header struct {
	Width     uint16
	Height    uint16
	TitleLen  uint8
	Title     [16]byte
	UpdateDate float64
	Reserved  [23]byte
}

// Cell is a 12-byte map cell (TMapUnitInfo).
type Cell struct {
	BkImg      uint16 // bit15 = collision, bits 0-14 = image index (1-based)
	MidImg     uint16
	FrImg      uint16
	DoorIndex  uint8  // bit7 = has door
	DoorOffset uint8  // bit7 = door open
	AniFrame   uint8  // bit7 = alpha blend, bits 6-0 = frame count
	AniTick    uint8
	Area       uint8  // selects Objects{N+1}.wil
	Light      uint8  // 0-4
}

// MapData holds the parsed map.
type MapData struct {
	Header Header
	Cells  []Cell
	Width  int
	Height int
}

// At returns the cell at (x, y) in row-major order.
func (m *MapData) At(x, y int) *Cell {
	return &m.Cells[y*m.Width+x]
}

// IsCollision returns true if the cell at (x, y) is blocked.
func (m *MapData) IsCollision(x, y int) bool {
	return (m.Cells[y*m.Width+x].BkImg & 0x8000) != 0
}

// Parse reads a .map file and returns MapData.
func Parse(path string) (*MapData, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	if len(data) < headerSize {
		return nil, fmt.Errorf("file too small: %d bytes", len(data))
	}

	var hdr Header
	r := bytes.NewReader(data)
	if err := binary.Read(r, binary.LittleEndian, &hdr); err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	width := int(hdr.Width)
	height := int(hdr.Height)
	totalCells := width * height

	// Detect cell size
	remaining := len(data) - headerSize
	if totalCells == 0 {
		return nil, fmt.Errorf("zero cells (%dx%d)", width, height)
	}

	detectedCellSize := 0
	switch {
	case remaining == totalCells*12:
		detectedCellSize = 12
	case remaining == totalCells*14:
		detectedCellSize = 14
	case remaining == totalCells*20:
		detectedCellSize = 20
	case remaining%totalCells == 0 && remaining/totalCells >= 12:
		detectedCellSize = remaining / totalCells
	default:
		return nil, fmt.Errorf("unknown format: %d bytes for %d cells", remaining, totalCells)
	}

	// Read cells: file stores column-major, convert to row-major
	cells := make([]Cell, totalCells)
	for col := 0; col < width; col++ {
		for row := 0; row < height; row++ {
			fileOff := headerSize + (col*height+row)*detectedCellSize
			arrayIdx := row*width + col
			raw := data[fileOff : fileOff+cellSize]
			cells[arrayIdx] = Cell{
				BkImg:      binary.LittleEndian.Uint16(raw[0:2]),
				MidImg:     binary.LittleEndian.Uint16(raw[2:4]),
				FrImg:      binary.LittleEndian.Uint16(raw[4:6]),
				DoorIndex:  raw[6],
				DoorOffset: raw[7],
				AniFrame:   raw[8],
				AniTick:    raw[9],
				Area:       raw[10],
				Light:      raw[11],
			}
		}
	}

	return &MapData{
		Header: hdr,
		Cells:  cells,
		Width:  width,
		Height: height,
	}, nil
}
