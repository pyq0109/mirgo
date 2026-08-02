package main

import (
	"fmt"
	"image"
	"image/png"
	"os"

	"github.com/go-gl/gl/v3.3-core/gl"

	mlog "github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/wil"
)

// WILRenderer 管理 WIL 图像的 GL 纹理缓存与导出。
type WILRenderer struct {
	WILFile  *wil.File
	texCache map[int]uint32 // 图片索引 -> GL 纹理
}

// NewWILRenderer 创建 WIL 图像渲染器。
func NewWILRenderer(wilFile *wil.File) *WILRenderer {
	return &WILRenderer{
		WILFile:  wilFile,
		texCache: make(map[int]uint32),
	}
}

// SetWILFile 替换当前 WIL 文件并清除纹理缓存。
func (r *WILRenderer) SetWILFile(f *wil.File) {
	oldCount := len(r.texCache)
	for _, tex := range r.texCache {
		gl.DeleteTextures(1, &tex)
	}
	r.WILFile = f
	r.texCache = make(map[int]uint32)
	if f != nil {
		mlog.Logf(mlog.LevelDebug, "Renderer", "SetWILFile: title=%s, images=%d, 清除旧纹理=%d", f.Title, f.Count, oldCount)
	} else {
		mlog.Logf(mlog.LevelDebug, "Renderer", "SetWILFile: nil, 清除旧纹理=%d", oldCount)
	}
}

// GetOrCreateTexture 返回指定图片索引的 GL 纹理，需要时创建并缓存。
func (r *WILRenderer) GetOrCreateTexture(idx int) uint32 {
	return r.getTexture(idx)
}

// getTexture 返回指定图片索引的 GL 纹理，需要时缓存。
func (r *WILRenderer) getTexture(idx int) uint32 {
	if idx < 0 || idx >= r.WILFile.Count {
		return 0
	}
	if tex, ok := r.texCache[idx]; ok {
		mlog.Logf(mlog.LevelTrace, "Renderer", "纹理缓存命中: idx=%d, tex=%d", idx, tex)
		return tex
	}
	img := r.WILFile.GetImage(idx)
	if img == nil || img.RGBA == nil {
		mlog.Logf(mlog.LevelWarn, "Renderer", "图像为空: idx=%d", idx)
		return 0
	}
	tex := UploadTexture(img.RGBA)
	r.texCache[idx] = tex
	mlog.Logf(mlog.LevelTrace, "Renderer", "纹理上传: idx=%d, size=%dx%d, tex=%d", idx, img.Width, img.Height, tex)
	return tex
}

// ExportPNG 将指定图像导出为 PNG 文件。
func (r *WILRenderer) ExportPNG(idx int, path string) error {
	if r.WILFile == nil || idx < 0 || idx >= r.WILFile.Count {
		mlog.Logf(mlog.LevelError, "Export", "导出失败: 无效索引 idx=%d", idx)
		return os.ErrInvalid
	}
	img := r.WILFile.GetImage(idx)
	if img == nil || img.RGBA == nil {
		mlog.Logf(mlog.LevelError, "Export", "导出失败: 图像为空 idx=%d", idx)
		return os.ErrInvalid
	}
	f, err := os.Create(path)
	if err != nil {
		mlog.Logf(mlog.LevelError, "Export", "创建文件失败: %s, err=%v", path, err)
		return err
	}
	defer f.Close()
	err = png.Encode(f, img.RGBA)
	if err != nil {
		mlog.Logf(mlog.LevelError, "Export", "PNG编码失败: idx=%d, err=%v", idx, err)
	} else {
		mlog.Logf(mlog.LevelInfo, "Export", "导出成功: idx=%d, size=%dx%d, path=%s", idx, img.Width, img.Height, path)
	}
	return err
}

// ExportAllPNG 将当前 WIL 文件的所有图像导出到目录。
func (r *WILRenderer) ExportAllPNG(dir string) (int, error) {
	if r.WILFile == nil {
		mlog.Logf(mlog.LevelError, "Export", "批量导出失败: 无WIL文件")
		return 0, os.ErrInvalid
	}
	mlog.Logf(mlog.LevelInfo, "Export", "批量导出开始: title=%s, images=%d, dir=%s", r.WILFile.Title, r.WILFile.Count, dir)
	exported := 0
	for i, img := range r.WILFile.Images {
		if img == nil || img.RGBA == nil {
			continue
		}
		path := dir + "/" + formatIdx(i) + ".png"
		f, err := os.Create(path)
		if err != nil {
			mlog.Logf(mlog.LevelError, "Export", "批量导出失败: idx=%d, err=%v", i, err)
			return exported, err
		}
		if err := png.Encode(f, img.RGBA); err != nil {
			f.Close()
			mlog.Logf(mlog.LevelError, "Export", "批量导出编码失败: idx=%d, err=%v", i, err)
			return exported, err
		}
		f.Close()
		exported++
	}
	mlog.Logf(mlog.LevelInfo, "Export", "批量导出完成: exported=%d", exported)
	return exported, nil
}

// formatIdx 将图片索引格式化为文件名（零填充）。
func formatIdx(i int) string {
	if i < 1000 {
		return fmt.Sprintf("%03d", i)
	}
	return fmt.Sprintf("%04d", i)
}

// Destroy 释放渲染器持有的所有 GL 资源。
func (r *WILRenderer) Destroy() {
	count := len(r.texCache)
	for _, tex := range r.texCache {
		gl.DeleteTextures(1, &tex)
	}
	mlog.Logf(mlog.LevelDebug, "Renderer", "Destroy: 清除纹理=%d", count)
}

// UploadTexture 将 *image.RGBA 上传为 OpenGL 纹理。
func UploadTexture(img *image.RGBA) uint32 {
	var tex uint32
	gl.GenTextures(1, &tex)
	gl.BindTexture(gl.TEXTURE_2D, tex)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA,
		int32(img.Bounds().Dx()), int32(img.Bounds().Dy()),
		0, gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(img.Pix))
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	gl.BindTexture(gl.TEXTURE_2D, 0)
	return tex
}
