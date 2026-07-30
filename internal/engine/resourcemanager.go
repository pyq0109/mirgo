package engine

import (
	"fmt"
	"image"
	"path/filepath"
	"sync"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/wil"
)

var texLogCount int

// ResourceManager 管理 WIL 文件加载与纹理缓存。
type ResourceManager struct {
	dataDir string
	gl      *GLState

	// WIL 文件
	Tiles    *wil.File
	SmTiles  *wil.File
	Objects  [15]*wil.File
	Hum      *wil.File
	Hair     *wil.File
	Weapon   *wil.File
	Mon      [28]*wil.File
	Npc      *wil.File
	Magic    *wil.File
	Magic2   *wil.File
	Items    *wil.File
	StateItem *wil.File
	DnItems  *wil.File
	Prguse   *wil.File
	Prguse2  *wil.File
	Prguse3  *wil.File
	ChrSel   *wil.File
	Mmap     *wil.File
	Effect   *wil.File
	Dragon   *wil.File
	Event    *wil.File
	HumEffect *wil.File
	MagIcon  *wil.File

	// 纹理缓存
	mu       sync.RWMutex
	texCache map[string]uint32 // "wilName:index" -> 纹理 ID

	// 懒加载的辅助 WIL（St<N>.wil 装备外观文件）；nil 值用于缓存
	// 加载失败的结果。
	extraMu sync.Mutex
	extras  map[string]*wil.File
}

// NewResourceManager 创建一个新的资源管理器并加载所有 WIL 文件。
func NewResourceManager(dataDir string, gl *GLState) (*ResourceManager, error) {
	rm := &ResourceManager{
		dataDir:  dataDir,
		gl:       gl,
		texCache: make(map[string]uint32),
	}

	if err := rm.loadAll(); err != nil {
		return nil, err
	}

	return rm, nil
}

// DataDir 返回数据目录路径。
func (rm *ResourceManager) DataDir() string {
	return rm.dataDir
}

func (rm *ResourceManager) loadAll() error {
	load := func(name string) (*wil.File, error) {
		path := filepath.Join(rm.dataDir, name)
		f, err := wil.Load(path)
		if err != nil {
			return nil, fmt.Errorf("load %s: %w", name, err)
		}
		return f, nil
	}

	var err error

	// 地图地砖
	rm.Tiles, err = load("Tiles.wil")
	if err != nil {
		return err
	}
	rm.SmTiles, err = load("SmTiles.wil")
	if err != nil {
		return err
	}

	// Objects（1-15）
	for i := 0; i < 15; i++ {
		name := fmt.Sprintf("Objects%d.wil", i+1)
		if i == 0 {
			name = "Objects.wil"
		}
		rm.Objects[i], _ = load(name) // 可选，忽略错误
	}

	// 角色资源
	rm.Hum, err = load("Hum.wil")
	if err != nil {
		return err
	}
	rm.Hair, err = load("Hair.wil")
	if err != nil {
		return err
	}
	rm.Weapon, err = load("Weapon.wil")
	if err != nil {
		return err
	}

	// 怪物文件（Mon1-Mon28，可选）
	for i := 0; i < 28; i++ {
		name := fmt.Sprintf("Mon%d.wil", i+1)
		rm.Mon[i], _ = load(name)
	}

	// NPC
	rm.Npc, err = load("Npc.wil")
	if err != nil {
		return err
	}

	// 魔法特效
	rm.Magic, _ = load("Magic.wil")
	rm.Magic2, _ = load("Magic2.wil")

	// 物品
	rm.Items, err = load("Items.wil")
	if err != nil {
		return err
	}
	rm.StateItem, err = load("StateItem.wil")
	if err != nil {
		return err
	}
	rm.DnItems, err = load("DnItems.wil")
	if err != nil {
		return err
	}

	// UI
	rm.Prguse, err = load("Prguse.wil")
	if err != nil {
		return err
	}
	rm.Prguse2, _ = load("Prguse2.wil")
	rm.Prguse3, _ = load("Prguse3.wil")

	// 角色选择
	rm.ChrSel, err = load("ChrSel.wil")
	if err != nil {
		return err
	}

	// 小地图
	rm.Mmap, _ = load("mmap.wil")

	// 特效
	rm.Effect, _ = load("Effect.wil")
	rm.Dragon, _ = load("Dragon.wil")
	rm.Event, _ = load("Event.wil")
	rm.HumEffect, _ = load("HumEffect.wil")
	rm.MagIcon, _ = load("MagIcon.wil")

	return nil
}

// GetTexture 返回给定 WIL 文件和图像索引对应的缓存纹理。
func (rm *ResourceManager) GetTexture(f *wil.File, index int) uint32 {
	if f == nil || index < 0 || index >= f.Count {
		return 0
	}

	key := fmt.Sprintf("%p:%d", f, index)

	rm.mu.RLock()
	if tex, ok := rm.texCache[key]; ok {
		rm.mu.RUnlock()
		return tex
	}
	rm.mu.RUnlock()

	img := f.GetImage(index)
	if img == nil || img.RGBA == nil {
		log.Logf(log.LevelWarn, "GL", "GetTexture MISS file=%s idx=%d img=%v rgba=%v", f.Title, index, img != nil, img != nil && img.RGBA != nil)
		return 0
	}

	tex := rm.gl.UploadTexture(img.RGBA)
	texLogCount++
	if texLogCount <= 30 {
		iw := img.Width
		midRow := img.Height / 2
		midSample := ""
		if midRow < img.Height {
			rowOff := midRow * iw * 4
			for x := 0; x < iw && x < 8; x++ {
				off := rowOff + x*4
				midSample += fmt.Sprintf("(%d,%d,%d,%d) ", img.RGBA.Pix[off], img.RGBA.Pix[off+1], img.RGBA.Pix[off+2], img.RGBA.Pix[off+3])
			}
		}
		log.Logf(log.LevelInfo, "GL", "GetTexture #%d file=%s idx=%d %dx%d tex=%d midRow_RGBA=[%s]",
			texLogCount, f.Title, index, img.Width, img.Height, tex, midSample)
	}

	rm.mu.Lock()
	rm.texCache[key] = tex
	rm.mu.Unlock()

	return tex
}

// GetImage 返回给定 WIL 文件和索引对应的原始图像。
func (rm *ResourceManager) GetImage(f *wil.File, index int) *image.RGBA {
	if f == nil || index < 0 || index >= f.Count {
		return nil
	}
	img := f.GetImage(index)
	if img == nil {
		return nil
	}
	return img.RGBA
}

// GetExtraWil 按文件名懒加载一个辅助 WIL（如 "St1.wil"，用于装备
// Looks >= 10000 的情况；ClMain.pas:6179-6210）。文件不存在时返回 nil。
func (rm *ResourceManager) GetExtraWil(name string) *wil.File {
	rm.extraMu.Lock()
	defer rm.extraMu.Unlock()
	if rm.extras == nil {
		rm.extras = make(map[string]*wil.File)
	}
	if f, ok := rm.extras[name]; ok {
		return f
	}
	f, err := wil.Load(filepath.Join(rm.dataDir, name))
	if err != nil {
		rm.extras[name] = nil
		return nil
	}
	rm.extras[name] = f
	return f
}

// ClearCache 清空纹理缓存。
func (rm *ResourceManager) ClearCache() {
	rm.mu.Lock()
	for _, tex := range rm.texCache {
		rm.gl.DeleteTexture(tex)
	}
	rm.texCache = make(map[string]uint32)
	rm.mu.Unlock()
}

// Destroy 释放所有资源。
func (rm *ResourceManager) Destroy() {
	rm.ClearCache()
	rm.closeAllWils()
}

func (rm *ResourceManager) closeAllWils() {
	files := []*wil.File{
		rm.Tiles, rm.SmTiles, rm.Hum, rm.Hair, rm.Weapon, rm.Npc,
		rm.Magic, rm.Magic2, rm.Items, rm.StateItem, rm.DnItems,
		rm.Prguse, rm.Prguse2, rm.Prguse3, rm.ChrSel, rm.Mmap,
		rm.Effect, rm.Dragon, rm.Event, rm.HumEffect, rm.MagIcon,
	}
	for _, f := range files {
		if f != nil {
			f.Close()
		}
	}
	for _, f := range rm.Objects {
		if f != nil {
			f.Close()
		}
	}
	for _, f := range rm.Mon {
		if f != nil {
			f.Close()
		}
	}
	rm.extraMu.Lock()
	for _, f := range rm.extras {
		if f != nil {
			f.Close()
		}
	}
	rm.extras = nil
	rm.extraMu.Unlock()
}
