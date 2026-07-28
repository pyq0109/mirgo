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

	// 对应 C++ kLibTiles/kLibSmTiles/kLibObjects 的 Lib 索引常量。
	LibTiles    = 0 // Tiles.wil - 背景层
	LibSmTiles  = 1 // SmTiles.wil - 中间层
	LibObjects  = 2 // Objects.wil - 前景层
)

// Header 是 52 字节的地图文件头。
type Header struct {
	Width     uint16
	Height    uint16
	TitleLen  uint8
	Title     [16]byte
	UpdateDate float64
	Reserved  [23]byte
}

// Cell 是 12 字节的地图格子（TMapUnitInfo）。
type Cell struct {
	BkImg      uint16 // bit15 = 碰撞，bits 0-14 = 图像索引（从 1 开始）
	MidImg     uint16
	FrImg      uint16
	DoorIndex  uint8  // bit7 = 有门
	DoorOffset uint8  // bit7 = 门已打开
	AniFrame   uint8  // bit7 = alpha 混合，bits 6-0 = 帧数
	AniTick    uint8
	Area       uint8  // 选择 Objects{N+1}.wil
	Light      uint8  // 0-4
}

// CellInfo 是解析后的格子，lib/图像索引已分离。
// 对应 C++ common/map_types.h 中的 CellInfo。
type CellInfo struct {
	BackLib         int  // LibTiles，空则为 -1
	BackImage       int  // Tiles.wil 中从 0 开始的索引
	Collision       bool // (wBkImg & 0x8000) != 0
	MiddleLib       int  // LibSmTiles，空则为 -1
	MiddleImage     int  // SmTiles.wil 中从 0 开始的索引
	FrontLib        int  // LibObjects，空则为 -1
	FrontImage      int  // Objects{area+1}.wil 中从 0 开始的索引
	FrontArea       uint8
	FrontAniFrame   uint8 // bit7=alpha 混合，bits6-0=帧数
	FrontAniTick    uint8
	FrontDoorOffset uint8 // bit7=门已打开，bits6-0=偏移
	FrontDoorIndex  uint8 // bit7=有门，bits6-0=门组 id
	Door            uint8
	Light           uint8
}

// MapData 保存解析后的地图。
type MapData struct {
	Header    Header
	Cells     []Cell
	CellInfos []CellInfo
	Width     int
	Height    int
}

// At 按行优先返回 (x, y) 处的原始格子。
func (m *MapData) At(x, y int) *Cell {
	return &m.Cells[y*m.Width+x]
}

// InfoAt 按行优先返回 (x, y) 处解析后的格子信息。
func (m *MapData) InfoAt(x, y int) *CellInfo {
	return &m.CellInfos[y*m.Width+x]
}

// IsCollision 在 (x, y) 处的格子被阻挡时返回 true（仅地形，忽略门）。
func (m *MapData) IsCollision(x, y int) bool {
	return m.CellInfos[y*m.Width+x].Collision
}

// CanMove 在 (x, y) 处的格子可通行（考虑门）时返回 true。
// 对应 Delphi TMap.CanMove（MapUnit.pas:311-327）。
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

// CanFly 在 (x, y) 处的格子允许飞行时返回 true（仅检查 FrImg bit15）。
// 对应 Delphi TMap.CanFly（MapUnit.pas:329-343）。
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

// GetDoor 返回 (x, y) 处的门组 ID，无门则返回 0。
// 对应 Delphi TMap.GetDoor（MapUnit.pas:346-356）。
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

// IsDoorOpen 在 (x, y) 处的门已打开时返回 true。
// 对应 Delphi TMap.IsDoorOpen（MapUnit.pas:358-368）。
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

// OpenDoor 打开与 (x, y) 同属一个门组的所有格子。
// 对应 Delphi TMap.OpenDoor（MapUnit.pas:370-387）。
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

// CloseDoor 关闭与 (x, y) 同属一个门组的所有格子。
// 对应 Delphi TMap.CloseDoor（MapUnit.pas:389-405）。
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

// Parse 读取一个 .map 文件并返回 MapData。
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

	// 检测格子大小
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

	// 读取格子：文件按列优先存储，转换为行优先
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

// parseCells 将原始 Cell 数据转换为 lib/图像索引已分离的 CellInfo。
// 对应 C++ common/map_parser.cpp 中的 ParseCells。
func (m *MapData) parseCells() {
	total := len(m.Cells)
	m.CellInfos = make([]CellInfo, total)
	for i := 0; i < total; i++ {
		raw := &m.Cells[i]
		info := &m.CellInfos[i]

		// 碰撞：Delphi CanMove 同时检查 wBkImg 和 wFrImg 的 bit15（MapUnit.pas:320）
		info.Collision = (raw.BkImg&0x8000) != 0 || (raw.FrImg&0x8000) != 0
		backImg := int(raw.BkImg&0x7FFF) - 1
		if backImg >= 0 {
			info.BackLib = LibTiles
			info.BackImage = backImg
		} else {
			info.BackLib = -1
			info.BackImage = -1
		}

		// 中间层：bits 0-14 = 从 1 开始的图像索引
		midImg := int(raw.MidImg&0x7FFF) - 1
		if midImg >= 0 {
			info.MiddleLib = LibSmTiles
			info.MiddleImage = midImg
		} else {
			info.MiddleLib = -1
			info.MiddleImage = -1
		}

		// 前景层：bits 0-14 = 从 1 开始的图像索引
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

		// 前景层动画/alpha/门的元数据
		if frontImg >= 0 {
			info.FrontArea = raw.Area
			info.FrontAniFrame = raw.AniFrame
			info.FrontAniTick = raw.AniTick
			info.FrontDoorOffset = raw.DoorOffset
			info.FrontDoorIndex = raw.DoorIndex
		}
	}
}
