package renderer

import (
	"image"
	"image/png"
	"os"

	"github.com/go-gl/gl/v3.3-core/gl"

	mlog "github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/wil"
)

// WILRenderer 使用 OpenGL 渲染 WIL 图像。
type WILRenderer struct {
	WILFile  *wil.File
	glState  *GLState
	texCache map[int]uint32 // 图片索引 -> GL 纹理

	// 相机状态，用于缩放/平移
	Zoom             float64
	OffsetX, OffsetY float64
}

// NewWILRenderer 创建 WIL 图像渲染器。
func NewWILRenderer(wilFile *wil.File, glState *GLState) *WILRenderer {
	return &WILRenderer{
		WILFile:  wilFile,
		glState:  glState,
		texCache: make(map[int]uint32),
		Zoom:     1.0,
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
	r.Zoom = 1.0
	r.OffsetX = 0
	r.OffsetY = 0
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

// GetImage 返回指定索引的图像数据，不可用时返回 nil。
func (r *WILRenderer) GetImage(idx int) *wil.Image {
	if r.WILFile == nil || idx < 0 || idx >= r.WILFile.Count {
		return nil
	}
	return r.WILFile.GetImage(idx)
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

// Render 在指定视口中绘制 WIL 图像。
// vpX, vpY, vpW, vpH 定义屏幕像素坐标的 GL 视口。
func (r *WILRenderer) Render(idx int, vpX, vpY, vpW, vpH int32) {
	if r.WILFile == nil || idx < 0 || idx >= r.WILFile.Count {
		return
	}

	tex := r.getTexture(idx)
	if tex == 0 {
		return
	}

	img := r.WILFile.GetImage(idx)
	if img == nil || img.Width <= 0 || img.Height <= 0 {
		return
	}

	// 设置视口到中心区域
	gl.Viewport(vpX, vpY, vpW, vpH)

	// 启用透明图像混合
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)

	// 使用着色器程序
	gl.UseProgram(r.glState.Shader.ID)
	gl.Uniform1i(r.glState.Shader.TexLoc, 0)

	// 正交投影：像素坐标，Y 轴向下
	imgW := float32(img.Width)
	imgH := float32(img.Height)
	vpw := float32(vpW)
	vph := float32(vpH)
	zoom := float32(r.Zoom)

	// 视口中心的世界坐标
	cx := vpw/2/zoom + float32(r.OffsetX)
	cy := vph/2/zoom + float32(r.OffsetY)

	// 图像位置：居中于 (cx, cy)
	x := cx - imgW/2
	y := cy - imgH/2

	proj := OrthoProj(float32(r.OffsetX), vpw/zoom+float32(r.OffsetX), vph/zoom+float32(r.OffsetY), float32(r.OffsetY))
	r.glState.DrawQuad(x, y, imgW, imgH, tex, true, proj)
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

func formatIdx(i int) string {
	if i < 10 {
		return "000" + string(rune('0'+i))
	}
	if i < 100 {
		return "00" + itoa(i)
	}
	if i < 1000 {
		return "0" + itoa(i)
	}
	return itoa(i)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	s := ""
	for i > 0 {
		s = string(rune('0'+i%10)) + s
		i /= 10
	}
	return s
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
