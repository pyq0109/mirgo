package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"unsafe"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/pyq0109/mirgo/internal/engine"
	"github.com/pyq0109/mirgo/internal/log"
)

type FogTexture struct {
	Width  int32
	Height int32
}

type LightingSystem struct {
	gl       *engine.GLState
	fogs     [6]*FogTexture
	fogTexID [6]uint32
	loaded   bool
}

func NewLightingSystem(glState *engine.GLState, dataDir string) *LightingSystem {
	ls := &LightingSystem{gl: glState}
	ls.load(dataDir)
	return ls
}

func (ls *LightingSystem) load(dataDir string) {
	names := []string{"lig0a.dat", "lig0b.dat", "lig0c.dat", "lig0d.dat", "lig0e.dat", "lig0f.dat"}
	for i, name := range names {
		path := filepath.Join(dataDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			log.Logf(log.LevelWarn, "Lighting", "failed to load %s: %v", name, err)
			continue
		}
		if len(data) < 8 {
			continue
		}
		w := int32(binary.LittleEndian.Uint32(data[0:4]))
		h := int32(binary.LittleEndian.Uint32(data[4:8]))
		expected := int(w * h)
		if expected <= 0 || len(data) < 8+expected {
			continue
		}
		ls.fogs[i] = &FogTexture{Width: w, Height: h}

		rgba := make([]byte, expected*4)
		for j := 0; j < expected; j++ {
			rgba[j*4+0] = 255
			rgba[j*4+1] = 255
			rgba[j*4+2] = 255
			rgba[j*4+3] = data[8+j]
		}

		var tex uint32
		gl.GenTextures(1, &tex)
		gl.BindTexture(gl.TEXTURE_2D, tex)
		gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA, w, h, 0, gl.RGBA, gl.UNSIGNED_BYTE, unsafe.Pointer(&rgba[0]))
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
		gl.BindTexture(gl.TEXTURE_2D, 0)
		ls.fogTexID[i] = tex

		log.Logf(log.LevelInfo, "Lighting", "loaded %s: %dx%d", name, w, h)
	}
	ls.loaded = true
}

func (ls *LightingSystem) Render(proj [16]float32, camX, camY float64, viewW, viewH int, zoom float64, darkness float32, lightSources []LightSource) {
	if darkness <= 0.01 {
		return
	}

	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	w := float32(float64(viewW) / zoom)
	h := float32(float64(viewH) / zoom)
	ls.gl.DrawQuadColor(float32(camX), float32(camY), w, h, 0, 0, 0, darkness, proj)

	if len(lightSources) > 0 {
		gl.BlendFunc(gl.SRC_ALPHA, gl.ONE)
		for _, src := range lightSources {
			level := src.Level
			if level < 0 {
				level = 0
			}
			if level > 5 {
				level = 5
			}
			tex := ls.fogTexID[level]
			if tex == 0 || ls.fogs[level] == nil {
				continue
			}
			fog := ls.fogs[level]
			halfW := float32(fog.Width) / 2
			halfH := float32(fog.Height) / 2
			wx := float32(src.X) - halfW
			wy := float32(src.Y) - halfH
			ls.gl.DrawQuad(tex, wx, wy, float32(fog.Width), float32(fog.Height), proj)
		}
		gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	}
}

func (ls *LightingSystem) Destroy() {
	for i := range ls.fogTexID {
		if ls.fogTexID[i] != 0 {
			gl.DeleteTextures(1, &ls.fogTexID[i])
			ls.fogTexID[i] = 0
		}
	}
}
