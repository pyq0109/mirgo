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

	// Lib index constants matching C++ kLibTiles/kLibSmTiles/kLibObjects.
	LibTiles    = 0 // Tiles.wil - background
	LibSmTiles  = 1 // SmTiles.wil - middle layer
	LibObjects  = 2 // Objects.wil - foreground
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

// CellInfo is the parsed cell with separated lib/image indices.
// Matches C++ CellInfo from common/map_types.h.
type CellInfo struct {
	BackLib         int  // LibTiles or -1 if empty
	BackImage       int  // 0-based index into Tiles.wil
	Collision       bool // (wBkImg & 0x8000) != 0
	MiddleLib       int  // LibSmTiles or -1 if empty
	MiddleImage     int  // 0-based index into SmTiles.wil
	FrontLib        int  // LibObjects or -1 if empty
	FrontImage      int  // 0-based index into Objects{area+1}.wil
	FrontArea       uint8
	FrontAniFrame   uint8 // bit7=alpha blend, bits6-0=frame count
	FrontAniTick    uint8
	FrontDoorOffset uint8 // bit7=door open, bits6-0=offset
	FrontDoorIndex  uint8 // bit7=has door, bits6-0=door group id
	Door            uint8
	Light           uint8
}

// MapData holds the parsed map.
type MapData struct {
	Header    Header
	Cells     []Cell
	CellInfos []CellInfo
	Width     int
	Height    int
}

// At returns the raw cell at (x, y) in row-major order.
func (m *MapData) At(x, y int) *Cell {
	return &m.Cells[y*m.Width+x]
}

// InfoAt returns the parsed cell info at (x, y) in row-major order.
func (m *MapData) InfoAt(x, y int) *CellInfo {
	return &m.CellInfos[y*m.Width+x]
}

// IsCollision returns true if the cell at (x, y) is blocked (terrain only, ignores doors).
func (m *MapData) IsCollision(x, y int) bool {
	return m.CellInfos[y*m.Width+x].Collision
}

// CanMove returns true if the cell at (x, y) is walkable, considering doors.
// Matches Delphi TMap.CanMove (MapUnit.pas:311-327).
func (m *MapData) CanMove(x, y int) bool {
	if x < 0 || x >= m.Width || y < 0 || y >= m.Height {
		return false
	}
	idx := y*m.Width + x
	if m.CellInfos[idx].Collision {
		return false
	}
	cell := &m.Cells[idx]
	if cell.DoorIndex&0x80 != 0 {
		if cell.DoorOffset&0x80 == 0 {
			return false
		}
	}
	return true
}

// CanFly returns true if the cell at (x, y) allows flight (only checks FrImg bit15).
// Matches Delphi TMap.CanFly (MapUnit.pas:329-343).
func (m *MapData) CanFly(x, y int) bool {
	if x < 0 || x >= m.Width || y < 0 || y >= m.Height {
		return false
	}
	idx := y*m.Width + x
	if m.Cells[idx].FrImg&0x8000 != 0 {
		return false
	}
	cell := &m.Cells[idx]
	if cell.DoorIndex&0x80 != 0 {
		if cell.DoorOffset&0x80 == 0 {
			return false
		}
	}
	return true
}

// GetDoor returns the door group ID at (x, y), or 0 if no door.
// Matches Delphi TMap.GetDoor (MapUnit.pas:346-356).
func (m *MapData) GetDoor(x, y int) int {
	if x < 0 || x >= m.Width || y < 0 || y >= m.Height {
		return 0
	}
	cell := &m.Cells[y*m.Width+x]
	if cell.DoorIndex&0x80 != 0 {
		return int(cell.DoorIndex & 0x7F)
	}
	return 0
}

// IsDoorOpen returns true if the door at (x, y) is open.
// Matches Delphi TMap.IsDoorOpen (MapUnit.pas:358-368).
func (m *MapData) IsDoorOpen(x, y int) bool {
	if x < 0 || x >= m.Width || y < 0 || y >= m.Height {
		return false
	}
	cell := &m.Cells[y*m.Width+x]
	if cell.DoorIndex&0x80 != 0 {
		return cell.DoorOffset&0x80 != 0
	}
	return false
}

// OpenDoor opens all cells belonging to the same door group as (x, y).
// Matches Delphi TMap.OpenDoor (MapUnit.pas:370-387).
func (m *MapData) OpenDoor(x, y int) {
	if x < 0 || x >= m.Width || y < 0 || y >= m.Height {
		return
	}
	idx := y*m.Width + x
	cell := &m.Cells[idx]
	if cell.DoorIndex&0x80 == 0 {
		return
	}
	doorID := cell.DoorIndex & 0x7F
	for dy := y - 10; dy <= y+10; dy++ {
		for dx := x - 10; dx <= x+10; dx++ {
			if dx < 0 || dx >= m.Width || dy < 0 || dy >= m.Height {
				continue
			}
			c := &m.Cells[dy*m.Width+dx]
			if c.DoorIndex&0x7F == doorID {
				c.DoorOffset |= 0x80
			}
		}
	}
}

// CloseDoor closes all cells belonging to the same door group as (x, y).
// Matches Delphi TMap.CloseDoor (MapUnit.pas:389-405).
func (m *MapData) CloseDoor(x, y int) {
	if x < 0 || x >= m.Width || y < 0 || y >= m.Height {
		return
	}
	idx := y*m.Width + x
	cell := &m.Cells[idx]
	if cell.DoorIndex&0x80 == 0 {
		return
	}
	doorID := cell.DoorIndex & 0x7F
	for dy := y - 8; dy <= y+10; dy++ {
		for dx := x - 8; dx <= x+10; dx++ {
			if dx < 0 || dx >= m.Width || dy < 0 || dy >= m.Height {
				continue
			}
			c := &m.Cells[dy*m.Width+dx]
			if c.DoorIndex&0x7F == doorID {
				c.DoorOffset &= 0x7F
			}
		}
	}
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

	md := &MapData{
		Header: hdr,
		Cells:  cells,
		Width:  width,
		Height: height,
	}
	md.parseCells()
	return md, nil
}

// parseCells converts raw Cell data into CellInfo with separated lib/image indices.
// Matches C++ ParseCells from common/map_parser.cpp.
func (m *MapData) parseCells() {
	total := len(m.Cells)
	m.CellInfos = make([]CellInfo, total)
	for i := 0; i < total; i++ {
		raw := &m.Cells[i]
		info := &m.CellInfos[i]

		// Collision: Delphi CanMove checks both wBkImg and wFrImg bit15 (MapUnit.pas:320)
		info.Collision = (raw.BkImg&0x8000) != 0 || (raw.FrImg&0x8000) != 0
		backImg := int(raw.BkImg&0x7FFF) - 1
		if backImg >= 0 {
			info.BackLib = LibTiles
			info.BackImage = backImg
		} else {
			info.BackLib = -1
			info.BackImage = -1
		}

		// Middle layer: bits 0-14 = 1-based image index
		midImg := int(raw.MidImg&0x7FFF) - 1
		if midImg >= 0 {
			info.MiddleLib = LibSmTiles
			info.MiddleImage = midImg
		} else {
			info.MiddleLib = -1
			info.MiddleImage = -1
		}

		// Front layer: bits 0-14 = 1-based image index
		frontImg := int(raw.FrImg&0x7FFF) - 1
		if frontImg >= 0 {
			info.FrontLib = LibObjects
			info.FrontImage = frontImg
		} else {
			info.FrontLib = -1
			info.FrontImage = -1
		}

		info.Door = raw.DoorIndex
		info.Light = raw.Light

		// Front layer metadata for animation/alpha/door
		if frontImg >= 0 {
			info.FrontArea = raw.Area
			info.FrontAniFrame = raw.AniFrame
			info.FrontAniTick = raw.AniTick
			info.FrontDoorOffset = raw.DoorOffset
			info.FrontDoorIndex = raw.DoorIndex
		}
	}
}
