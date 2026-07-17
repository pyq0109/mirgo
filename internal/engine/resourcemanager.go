package engine

import (
	"fmt"
	"image"
	"path/filepath"
	"sync"

	"github.com/pyq0109/mirgo/internal/wil"
)

// ResourceManager manages WIL file loading and texture caching.
type ResourceManager struct {
	dataDir string
	gl      *GLState

	// WIL files
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

	// Texture cache
	mu       sync.RWMutex
	texCache map[string]uint32 // "wilName:index" -> texture ID
}

// NewResourceManager creates a new resource manager and loads all WIL files.
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

	// Map tiles
	rm.Tiles, err = load("Tiles.wil")
	if err != nil {
		return err
	}
	rm.SmTiles, err = load("SmTiles.wil")
	if err != nil {
		return err
	}

	// Objects (1-15)
	for i := 0; i < 15; i++ {
		name := fmt.Sprintf("Objects%d.wil", i+1)
		if i == 0 {
			name = "Objects.wil"
		}
		rm.Objects[i], _ = load(name) // Optional, ignore errors
	}

	// Character assets
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

	// Monster files (Mon1-Mon28, optional)
	for i := 0; i < 28; i++ {
		name := fmt.Sprintf("Mon%d.wil", i+1)
		rm.Mon[i], _ = load(name)
	}

	// NPC
	rm.Npc, err = load("Npc.wil")
	if err != nil {
		return err
	}

	// Magic effects
	rm.Magic, _ = load("Magic.wil")
	rm.Magic2, _ = load("Magic2.wil")

	// Items
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

	// Character selection
	rm.ChrSel, err = load("ChrSel.wil")
	if err != nil {
		return err
	}

	// Minimap
	rm.Mmap, _ = load("mmap.wil")

	// Effects
	rm.Effect, _ = load("Effect.wil")
	rm.Dragon, _ = load("Dragon.wil")
	rm.Event, _ = load("Event.wil")
	rm.HumEffect, _ = load("HumEffect.wil")
	rm.MagIcon, _ = load("MagIcon.wil")

	return nil
}

// GetTexture returns a cached texture for the given WIL file and image index.
func (rm *ResourceManager) GetTexture(f *wil.File, index int) uint32 {
	if f == nil || index < 0 || index >= len(f.Images) {
		return 0
	}

	key := fmt.Sprintf("%p:%d", f, index)

	rm.mu.RLock()
	if tex, ok := rm.texCache[key]; ok {
		rm.mu.RUnlock()
		return tex
	}
	rm.mu.RUnlock()

	// Load the image
	img := f.Images[index]
	if img == nil || img.RGBA == nil {
		return 0
	}

	tex := rm.gl.UploadTexture(img.RGBA)

	rm.mu.Lock()
	rm.texCache[key] = tex
	rm.mu.Unlock()

	return tex
}

// GetImage returns the raw image for the given WIL file and index.
func (rm *ResourceManager) GetImage(f *wil.File, index int) *image.RGBA {
	if f == nil || index < 0 || index >= len(f.Images) {
		return nil
	}
	return f.Images[index].RGBA
}

// ClearCache clears the texture cache.
func (rm *ResourceManager) ClearCache() {
	rm.mu.Lock()
	for _, tex := range rm.texCache {
		rm.gl.DeleteTexture(tex)
	}
	rm.texCache = make(map[string]uint32)
	rm.mu.Unlock()
}

// Destroy frees all resources.
func (rm *ResourceManager) Destroy() {
	rm.ClearCache()
}
