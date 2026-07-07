package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unsafe"

	"github.com/go-gl/glfw/v3.4/glfw"

	ig "github.com/AllenDang/cimgui-go/imgui"
	igglfw "github.com/AllenDang/cimgui-go/impl/glfw"
	igopengl3 "github.com/AllenDang/cimgui-go/impl/opengl3"

	"github.com/pyq0109/mirgo/cmd/wilviewer/renderer"
	"github.com/pyq0109/mirgo/internal/wil"
)

const (
	leftPanelWidth  = 250
	rightPanelWidth = 380
)

// UIState holds the shared state between the UI and the main loop.
type UIState struct {
	DataDir    string // root data directory
	WILFile    *wil.File
	Renderer   *renderer.WILRenderer
	CurrentIdx int
	Mode       string // "browse" or "animation"
}

// toImGuiWindow converts a go-gl/glfw Window to the cimgui-go GLFWwindow type.
func toImGuiWindow(w *glfw.Window) *igglfw.GLFWwindow {
	return igglfw.NewGLFWwindowFromC(unsafe.Pointer(w.Handle()))
}

// Init initializes ImGui with the given GLFW window.
func Init(window *glfw.Window) {
	ig.CreateContext()
	ig.StyleColorsDark()

	imWin := toImGuiWindow(window)
	igglfw.InitForOpenGL(imWin, true)
	igopengl3.InitV("#version 330")
}

// Shutdown shuts down ImGui backends and destroys the context.
func Shutdown() {
	igopengl3.Shutdown()
	igglfw.Shutdown()
	ig.DestroyContext()
}

// ScrollHandler is a callback for scroll events (after ImGui processing).
type ScrollHandler func(window *glfw.Window, xoff, yoff float64)

// SetGLFWCallbacks sets up GLFW callbacks that forward to ImGui.
// scrollHandler is called after ImGui processes the scroll event.
func SetGLFWCallbacks(window *glfw.Window, scrollHandler ScrollHandler) {
	imWin := toImGuiWindow(window)

	window.SetMouseButtonCallback(func(w *glfw.Window, button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) {
		igglfw.MouseButtonCallback(imWin, int32(button), int32(action), int32(mods))
	})

	window.SetCursorPosCallback(func(w *glfw.Window, xpos, ypos float64) {
		igglfw.CursorPosCallback(imWin, xpos, ypos)
	})

	window.SetScrollCallback(func(w *glfw.Window, xoff, yoff float64) {
		igglfw.ScrollCallback(imWin, xoff, yoff)
		if scrollHandler != nil {
			scrollHandler(w, xoff, yoff)
		}
	})

	window.SetKeyCallback(func(w *glfw.Window, key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) {
		igglfw.KeyCallback(imWin, int32(key), int32(scancode), int32(action), int32(mods))
	})
}

// BeginFrame starts a new ImGui frame.
func BeginFrame() {
	igopengl3.NewFrame()
	igglfw.NewFrame()
	ig.NewFrame()
}

// EndFrame finalizes and renders the ImGui draw data.
func EndFrame() {
	ig.Render()
	igopengl3.RenderDrawData(ig.CurrentDrawData())
}

// IO returns the current ImGui IO.
func IO() *ig.IO {
	return ig.CurrentIO()
}

// RenderMenuBar renders the top menu bar (File -> Exit).
func RenderMenuBar(shouldClose *bool) {
	if !ig.BeginMainMenuBar() {
		return
	}
	if ig.BeginMenu("File") {
		if ig.MenuItemBool("Exit") {
			*shouldClose = true
		}
		ig.EndMenu()
	}
	ig.EndMainMenuBar()
}

// FrameHeight returns the current ImGui frame height (menu bar height).
func FrameHeight() float32 {
	return ig.FrameHeight()
}

// RenderLeftPanel renders the directory tree panel on the left side.
func RenderLeftPanel(state *UIState, glfwW, glfwH int32, menuH float32) {
	ig.SetNextWindowPosV(ig.NewVec2(0, menuH), ig.CondAlways, ig.NewVec2(0, 0))
	ig.SetNextWindowSizeV(ig.NewVec2(leftPanelWidth, float32(glfwH)-menuH), ig.CondAlways)

	ig.BeginV("Files", nil, ig.WindowFlagsNoMove|ig.WindowFlagsNoResize)

	if state.DataDir == "" {
		ig.Text("No data directory")
		ig.End()
		return
	}

	ig.Text(filepath.Base(state.DataDir))
	ig.Separator()

	entries, err := os.ReadDir(state.DataDir)
	if err != nil {
		ig.Text("Error reading dir:")
		ig.Text(err.Error())
		ig.End()
		return
	}

	// Collect and sort .wil filenames.
	var wilFiles []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(strings.ToLower(name), ".wil") {
			wilFiles = append(wilFiles, name)
		}
	}
	sort.Strings(wilFiles)

	ig.BeginChildStr("filetree")
	for _, name := range wilFiles {
		selected := state.WILFile != nil && strings.EqualFold(state.WILFile.Title, strings.TrimSuffix(name, filepath.Ext(name)))
		if ig.SelectableBoolV(name, selected, 0, ig.NewVec2(0, 0)) {
			wilPath := filepath.Join(state.DataDir, name)
			newFile, err := wil.Load(wilPath)
			if err != nil {
				continue
			}
			state.WILFile = newFile
			state.CurrentIdx = 0
			state.Renderer.SetWILFile(newFile)
		}
	}
	ig.EndChild()

	ig.End()
}

// RenderMainPanel renders the main UI panel with image list and controls.
func RenderMainPanel(state *UIState, glfwW, glfwH int32, menuH float32) {
	ig.SetNextWindowPosV(ig.NewVec2(float32(glfwW-rightPanelWidth), menuH), ig.CondAlways, ig.NewVec2(0, 0))
	ig.SetNextWindowSizeV(ig.NewVec2(rightPanelWidth, float32(glfwH)-menuH), ig.CondAlways)

	ig.BeginV("WIL Info", nil, ig.WindowFlagsNoMove|ig.WindowFlagsNoResize)

	if state.WILFile == nil {
		ig.Text("Select a .wil file from")
		ig.Text("the left panel")
		ig.Separator()
		ig.Text("Controls:")
		ig.BulletText("Arrow keys: Navigate images")
		ig.BulletText("ESC: Quit")
		ig.End()
		return
	}

	wf := state.WILFile

	// File info.
	ig.Text(fmt.Sprintf("Title: %s", wf.Title))
	ig.Text(fmt.Sprintf("Images: %d", wf.Count))
	ig.Separator()

	// Mode selection.
	ig.Text("Mode:")
	if ig.RadioButtonBool("Browse", state.Mode == "browse") {
		state.Mode = "browse"
	}
	ig.SameLine()
	if ig.RadioButtonBool("Animation", state.Mode == "animation") {
		state.Mode = "animation"
	}
	ig.Separator()

	// Current image info.
	ig.Text("Current Image:")
	if state.CurrentIdx >= 0 && state.CurrentIdx < len(wf.Images) {
		img := wf.Images[state.CurrentIdx]
		if img != nil {
			ig.Text(fmt.Sprintf("  Index: %d", state.CurrentIdx))
			ig.Text(fmt.Sprintf("  Size: %d x %d", img.Width, img.Height))
			ig.Text(fmt.Sprintf("  Hotspot: (%d, %d)", img.HotX, img.HotY))
		}
	}
	ig.Separator()

	// Navigation.
	ig.Text("Navigation:")
	if ig.Button("<<") {
		if state.CurrentIdx > 0 {
			state.CurrentIdx--
		}
	}
	ig.SameLine()
	if ig.Button("<") {
		if state.CurrentIdx > 0 {
			state.CurrentIdx--
		}
	}
	ig.SameLine()
	ig.Text(fmt.Sprintf("%d / %d", state.CurrentIdx, wf.Count-1))
	ig.SameLine()
	if ig.Button(">") {
		if state.CurrentIdx < wf.Count-1 {
			state.CurrentIdx++
		}
	}
	ig.SameLine()
	if ig.Button(">>") {
		if state.CurrentIdx < wf.Count-1 {
			state.CurrentIdx++
		}
	}
	ig.Separator()

	// Image list (scrollable).
	ig.Text("Image List:")
	ig.BeginChildStr("imagelist")

	for i := 0; i < wf.Count; i++ {
		img := wf.Images[i]
		if img == nil {
			continue
		}

		label := fmt.Sprintf("%d: %dx%d", i, img.Width, img.Height)
		if ig.SelectableBool(label) {
			state.CurrentIdx = i
		}
	}

	ig.EndChild()

	ig.End()
}

// RightPanelWidth returns the width of the right panel for viewport calculations.
func RightPanelWidth() int {
	return rightPanelWidth
}
