package main

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"github.com/pyq0109/mirgo/internal/mapformat"
	"github.com/pyq0109/mirgo/internal/renderer"
	"github.com/pyq0109/mirgo/internal/wil"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "用法: mapviewer <map文件路径> [资源目录]")
		fmt.Fprintln(os.Stderr, "  map文件路径: .map 文件路径")
		fmt.Fprintln(os.Stderr, "  资源目录:    包含 Tiles.wil, SmTiles.wil, Objects.wil 的目录（默认 asset/client/Data）")
		os.Exit(1)
	}

	mapPath := os.Args[1]
	dataDir := "asset/client/Data"
	if len(os.Args) >= 3 {
		dataDir = os.Args[2]
	}

	// Parse map
	m, err := mapformat.Parse(mapPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "解析地图失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("地图: %dx%d, 标题: %s\n", m.Width, m.Height, string(m.Header.Title[:m.Header.TitleLen]))

	// Load WIL files
	fmt.Println("加载 Tiles.wil ...")
	tiles, err := wil.Load(filepath.Join(dataDir, "Tiles.wil"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载 Tiles.wil 失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("加载 SmTiles.wil ...")
	smTiles, err := wil.Load(filepath.Join(dataDir, "SmTiles.wil"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载 SmTiles.wil 失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("加载 Objects.wil ...")
	objects, err := wil.Load(filepath.Join(dataDir, "Objects.wil"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载 Objects.wil 失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Tiles: %d 张, SmTiles: %d 张, Objects: %d 张\n",
		tiles.Count, smTiles.Count, objects.Count)

	// Generate minimap
	minimapImg := renderer.GenerateMinimap(m)

	// Create renderer
	cam := renderer.NewCamera(1200, 800)
	ren := renderer.New(tiles, smTiles, objects, m.Width, m.Height)

	// Fyne app
	a := app.New()
	w := a.NewWindow("传奇地图查看器 - " + filepath.Base(mapPath))
	w.Resize(fyne.NewSize(1200, 800))

	// State
	showBack := true
	showMid := true
	showFront := true
	showCollision := false
	showGrid := false

	// Map image canvas
	mapCanvas := canvas.NewImageFromImage(image.NewRGBA(image.Rect(0, 0, 1, 1)))
	mapCanvas.FillMode = canvas.ImageFillStretch

	// Minimap canvas
	minimapCanvas := canvas.NewImageFromImage(minimapImg)
	minimapCanvas.FillMode = canvas.ImageFillStretch
	minimapCanvas.SetMinSize(fyne.NewSize(200, 200))

	// Info labels
	tileLabel := widget.NewLabel("格子: -")
	cellInfoLabel := widget.NewLabel("")
	mapInfoLabel := widget.NewLabel(fmt.Sprintf("地图: %dx%d", m.Width, m.Height))

	// Layer toggles
	backCheck := widget.NewCheck("背景层", func(v bool) { showBack = v })
	backCheck.SetChecked(true)
	midCheck := widget.NewCheck("中间层", func(v bool) { showMid = v })
	midCheck.SetChecked(true)
	frontCheck := widget.NewCheck("前景层", func(v bool) { showFront = v })
	frontCheck.SetChecked(true)
	collisionCheck := widget.NewCheck("碰撞", func(v bool) { showCollision = v })
	gridCheck := widget.NewCheck("网格", func(v bool) { showGrid = v })

	// Render function
	renderMap := func() {
		rendered := ren.Render(m, cam, showBack, showMid, showFront, showCollision, showGrid)
		mapCanvas.Image = rendered
		mapCanvas.Refresh()

		minimapViewport := renderer.GenerateMinimap(m)
		renderer.DrawMinimapViewport(minimapViewport, cam, m.Width, m.Height)
		minimapCanvas.Image = minimapViewport
		minimapCanvas.Refresh()
	}

	// Initial render
	renderMap()

	// Mouse interaction widget
	mapWidget := newMapWidget(cam, renderMap, func(tx, ty int) {
		if tx >= 0 && ty >= 0 && tx < m.Width && ty < m.Height {
			cell := m.At(tx, ty)
			tileLabel.SetText(fmt.Sprintf("格子: (%d, %d)", tx, ty))
			cellInfoLabel.SetText(fmt.Sprintf(
				"碰撞: %v\n光照: %d\n动画帧: %d (tick=%d)\nBlend: %v\n区域: %d\n门: %d",
				m.IsCollision(tx, ty),
				cell.Light,
				cell.AniFrame&0x7F, cell.AniTick,
				cell.AniFrame&0x80 != 0,
				cell.Area,
				cell.DoorIndex,
			))
		}
	})

	mapStack := container.NewMax(mapWidget, mapCanvas)

	minimapContainer := container.NewVBox(
		widget.NewLabel("小地图"),
		minimapCanvas,
	)

	infoPanel := container.NewVBox(
		mapInfoLabel,
		widget.NewSeparator(),
		tileLabel,
		cellInfoLabel,
		widget.NewSeparator(),
		backCheck,
		midCheck,
		frontCheck,
		collisionCheck,
		gridCheck,
		widget.NewSeparator(),
		widget.NewLabel("操作:"),
		widget.NewLabel("拖拽: 平移"),
		widget.NewLabel("滚轮: 缩放"),
		widget.NewLabel("左键: 查看格子"),
	)

	leftPanel := container.NewMax(
		mapStack,
		container.NewVBox(minimapContainer),
	)

	split := container.NewHSplit(leftPanel, infoPanel)
	split.SetOffset(0.8)

	w.SetContent(split)

	// Animation ticker
	ticker := time.NewTicker(100 * time.Millisecond)
	go func() {
		for range ticker.C {
			// TODO: update animation state
		}
	}()

	w.ShowAndRun()
	ticker.Stop()
}

// mapWidget handles mouse events for pan/zoom/click.
type mapWidget struct {
	widget.BaseWidget
	cam       *renderer.Camera2D
	onRender  func()
	onClick   func(tx, ty int)
	dragging  bool
	dragMoved bool
	lastDragX float64
	lastDragY float64
}

func newMapWidget(cam *renderer.Camera2D, onRender func(), onClick func(tx, ty int)) *mapWidget {
	mw := &mapWidget{
		cam:      cam,
		onRender: onRender,
		onClick:  onClick,
	}
	mw.ExtendBaseWidget(mw)
	return mw
}

func (mw *mapWidget) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(widget.NewLabel(""))
}

func (mw *mapWidget) Cursor() desktop.Cursor {
	return desktop.CrosshairCursor
}

func (mw *mapWidget) MouseDown(ev *desktop.MouseEvent) {
	if ev.Button == desktop.MouseButtonPrimary {
		mw.dragging = true
		mw.dragMoved = false
		mw.lastDragX = float64(ev.Position.X)
		mw.lastDragY = float64(ev.Position.Y)
	}
}

func (mw *mapWidget) MouseUp(ev *desktop.MouseEvent) {
	if ev.Button == desktop.MouseButtonPrimary {
		if !mw.dragMoved {
			wx, wy := mw.cam.ScreenToWorld(float64(ev.Position.X), float64(ev.Position.Y))
			tx, ty := mw.cam.WorldToTile(wx, wy)
			if mw.onClick != nil {
				mw.onClick(tx, ty)
			}
		}
		mw.dragging = false
	}
}

func (mw *mapWidget) MouseMoved(ev *desktop.MouseEvent) {
	if mw.dragging {
		dx := float64(ev.Position.X) - mw.lastDragX
		dy := float64(ev.Position.Y) - mw.lastDragY
		if dx != 0 || dy != 0 {
			mw.dragMoved = true
		}
		mw.cam.Pan(dx, dy)
		mw.lastDragX = float64(ev.Position.X)
		mw.lastDragY = float64(ev.Position.Y)
		if mw.onRender != nil {
			mw.onRender()
		}
	}
}

func (mw *mapWidget) Scrolled(ev *fyne.ScrollEvent) {
	factor := 1.0
	if ev.Scrolled.DY < 0 {
		factor = 1.1
	} else if ev.Scrolled.DY > 0 {
		factor = 0.9
	}
	mw.cam.ZoomAt(factor, float64(ev.Position.X), float64(ev.Position.Y))
	if mw.onRender != nil {
		mw.onRender()
	}
}
