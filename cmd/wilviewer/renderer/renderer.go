package renderer

import (
	"github.com/go-gl/gl/v3.3-core/gl"

	"github.com/pyq0109/mirgo/internal/wil"
)

// WILRenderer renders WIL images using OpenGL.
type WILRenderer struct {
	WILFile  *wil.File
	glState  *GLState
	texCache map[int]uint32 // image index -> GL texture
}

// NewWILRenderer creates a renderer for WIL images.
func NewWILRenderer(wilFile *wil.File, glState *GLState) *WILRenderer {
	return &WILRenderer{
		WILFile:  wilFile,
		glState:  glState,
		texCache: make(map[int]uint32),
	}
}

// SetWILFile replaces the current WIL file and clears the texture cache.
func (r *WILRenderer) SetWILFile(f *wil.File) {
	for _, tex := range r.texCache {
		gl.DeleteTextures(1, &tex)
	}
	r.WILFile = f
	r.texCache = make(map[int]uint32)
}

// getTexture returns a GL texture for the given image index, caching as needed.
func (r *WILRenderer) getTexture(idx int) uint32 {
	if idx < 0 || idx >= len(r.WILFile.Images) {
		return 0
	}
	if tex, ok := r.texCache[idx]; ok {
		return tex
	}
	img := r.WILFile.Images[idx]
	if img == nil || img.RGBA == nil {
		return 0
	}
	tex := UploadTexture(img.RGBA)
	r.texCache[idx] = tex
	return tex
}

// Render draws the specified WIL image centered in the viewport.
func (r *WILRenderer) Render(idx int) {
	if idx < 0 || idx >= len(r.WILFile.Images) {
		return
	}

	tex := r.getTexture(idx)
	if tex == 0 {
		return
	}

	img := r.WILFile.Images[idx]
	if img == nil || img.Width <= 0 || img.Height <= 0 {
		return
	}

	// Enable blending for transparent images.
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)

	// Use the shader program.
	gl.UseProgram(r.glState.Shader.ID)
	gl.Uniform1i(r.glState.Shader.TexLoc, 0)

	// Calculate projection matrix (orthographic, Y-down).
	// We want to center the image in the viewport.
	// For now, use a simple orthographic projection.
	proj := OrthoProj(0, 1, 1, 0)

	// Calculate image position to center it.
	// We'll use normalized coordinates and scale.
	imgW := float32(img.Width)
	imgH := float32(img.Height)

	// Scale to fit in viewport while maintaining aspect ratio.
	// For now, just draw at a fixed position.
	x := float32(0.25)
	y := float32(0.25)
	w := float32(0.5)
	h := float32(0.5)

	r.glState.DrawQuad(x, y, w, h, tex, true, proj)

	// Draw image info text (we'll add this later with ImGui).
	_ = imgW
	_ = imgH
}

// Destroy frees all GL resources held by the renderer.
func (r *WILRenderer) Destroy() {
	for _, tex := range r.texCache {
		gl.DeleteTextures(1, &tex)
	}
}
