package main

import (
	"github.com/pyq0109/mirgo/internal/engine"
	"github.com/pyq0109/mirgo/internal/log"
)

// NoticeScene 是 stLoginNotice 过渡画面。在 Delphi 中这是一个空场景
// (IntroScn.pas:1553-1561)——选角到进入游戏之间的黑屏，等待 RunLogin 完成。
type NoticeScene struct {
	gl *engine.GLState
}

func NewNoticeScene(gl *engine.GLState, resources *engine.ResourceManager, text *engine.TextRenderer) *NoticeScene {
	return &NoticeScene{gl: gl}
}

func (s *NoticeScene) Open() {
	log.Logf(log.LevelInfo, "NoticeScene", "opened")
}

func (s *NoticeScene) Close() {
	log.Logf(log.LevelInfo, "NoticeScene", "closed")
}

func (s *NoticeScene) Update(dt float64) {}

func (s *NoticeScene) Render(gl *engine.GLState, proj [16]float32) {
	gl.DrawQuadColor(0, 0, float32(ScreenWidth), float32(ScreenHeight), 0, 0, 0, 1, proj)
}

func (s *NoticeScene) OnKey(key int, action int) {}

func (s *NoticeScene) OnMouse(x, y float64, button int, action int, mods int) {}

func (s *NoticeScene) OnScroll(x, y float64) {}
