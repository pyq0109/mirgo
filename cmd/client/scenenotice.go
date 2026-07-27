package main

import (
	"github.com/pyq0109/mirgo/internal/engine"
	"github.com/pyq0109/mirgo/internal/log"
)

// NoticeScene is the stLoginNotice transition screen. In Delphi this is an
// empty scene (IntroScn.pas:1553-1561) — just a black screen between
// character select and gameplay while RunLogin completes.
type NoticeScene struct {
	gl *engine.GLState
}

func NewNoticeScene(gl *engine.GLState, resources *engine.ResourceManager, text *engine.TextRenderer) *NoticeScene {
	return &NoticeScene{gl: gl}
}

func (s *NoticeScene) Open() {
	log.Logf(log.LevelInfo, "NoticeScene", "Opened")
}

func (s *NoticeScene) Close() {
	log.Logf(log.LevelInfo, "NoticeScene", "Closed")
}

func (s *NoticeScene) Update(dt float64) {}

func (s *NoticeScene) Render(gl *engine.GLState, proj [16]float32) {
	gl.DrawQuadColor(0, 0, ScreenWidth, ScreenHeight, 0, 0, 0, 1, proj)
}

func (s *NoticeScene) OnKey(key int, action int) {}

func (s *NoticeScene) OnMouse(x, y float64, button int, action int, mods int) {}

func (s *NoticeScene) OnScroll(x, y float64) {}
