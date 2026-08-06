package main

import (
	"strings"

	"github.com/pyq0109/mirgo/internal/log"
)

// 本文件集中了 PlayScene 专有的调试命令注册。
// 场景进入 (Open) 时注册命令, 离开 (Close) 时注销。

// registerDebugCmds 在场景变为活动时向全局控制台注册命令。
func (s *PlayScene) registerDebugCmds() {
	dc := s.dbg
	dc.Register("panel", "panel <name> [on|off] — toggle UI panel", func(args []string) {
		s.cmdPanel(args)
	})
	dc.Register("key", "key <name> — simulate shortcut (b/c/m/enter/esc/f1-f12/1-6)", func(args []string) {
		s.cmdKey(args)
	})
}

// unregisterDebugCmds 在场景变为非活动时注销命令。
func (s *PlayScene) unregisterDebugCmds() {
	dc := s.dbg
	for _, name := range []string{"panel", "key"} {
		dc.Unregister(name)
	}
}

// cmdPanel 操作 GameState 中的面板可见性标志。
func (s *PlayScene) cmdPanel(args []string) {
	dc := s.dbg
	if len(args) == 0 {
		dc.Printf("usage: panel <name> [on|off]")
		dc.Printf("  names: bag state guild group friend abil npc shop deal minimap")
		return
	}
	log.Logf(log.LevelInfo, uiLogTag, "> panel %s", strings.Join(args, " "))
	name := strings.ToLower(args[0])
	var flag *bool
	switch name {
	case "bag":
		flag = &s.State.ShowBag
	case "state", "equip":
		flag = &s.State.ShowEquip
	case "guild":
		flag = &s.State.ShowGuild
	case "group":
		flag = &s.State.ShowGroupDlg
	case "friend":
		flag = &s.State.ShowFriend
	case "abil":
		flag = &s.State.ShowPlusAbil
	case "npc":
		flag = &s.State.ShowNpcDialog
	case "shop":
		flag = &s.State.ShowShop
	case "deal":
		flag = &s.State.InDeal
	case "minimap":
		// 小地图为三态（minimapLv 0/1/2），单独处理：
		// on 触发请求并置 Lv=1（不节流，调试用），off 直接隐藏。
		if len(args) >= 2 {
			switch strings.ToLower(args[1]) {
			case "on":
				if s.minimapLv == 0 {
					if s.sendWantMinimap != nil {
						s.sendWantMinimap()
					}
					s.minimapLv = 1
				}
			case "off":
				s.minimapLv = 0
			}
		} else {
			s.toggleMinimap()
		}
		dc.uiLogf("panel minimap lv = %d", s.minimapLv)
		return
	default:
		dc.uiLogf("unknown panel: %s", name)
		return
	}
	if len(args) >= 2 {
		switch strings.ToLower(args[1]) {
		case "on":
			*flag = true
		case "off":
			*flag = false
		}
	} else {
		*flag = !*flag
	}
	dc.uiLogf("panel %s = %s", name, onOff(*flag))
}

// cmdKey 模拟键盘快捷键。
func (s *PlayScene) cmdKey(args []string) {
	dc := s.dbg
	if len(args) == 0 {
		dc.Printf("usage: key <name>  (b c e g m n s v w enter esc f1-f12 1-6)")
		return
	}
	log.Logf(log.LevelInfo, uiLogTag, "> key %s", strings.Join(args, " "))
	keyMap := map[string]int{
		"b": 66, "c": 67, "e": 69, "g": 71, "m": 77,
		"n": 78, "s": 83, "v": 86, "w": 87, "z": 90,
		"enter": 257, "esc": 256,
		"f1": 290, "f2": 291, "f3": 292, "f4": 293,
		"f5": 294, "f6": 295, "f7": 296, "f8": 297,
		"f9": 298, "f10": 299, "f11": 300, "f12": 301,
		"1": 49, "2": 50, "3": 51, "4": 52, "5": 53, "6": 54,
	}
	name := strings.ToLower(args[0])
	code, ok := keyMap[name]
	if !ok {
		dc.uiLogf("unknown key: %s", name)
		return
	}
	s.OnKey(code, 1)
	dc.uiLogf("key %s -> OnKey(%d)", name, code)
}
