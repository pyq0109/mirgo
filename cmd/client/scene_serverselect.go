package main

import (
	"fmt"

	"github.com/pyq0109/mirgo/internal/engine"
	"github.com/pyq0109/mirgo/internal/log"
)

// serverInfo holds a server entry from SM_PASSOKSELECTSERVER.
type serverInfo struct {
	Name   string
	Status int
}

// SelectServerScene handles the server selection dialog (stSelectCountry).
// Layout matches Delphi FState.pas DSelServerDlg (English version).
type SelectServerScene struct {
	gl        *engine.GLState
	resources *engine.ResourceManager
	text      *engine.TextRenderer

	// Server list
	servers  []serverInfo
	selected int

	// Callbacks
	selectFunc func(serverName string)
	closeFunc  func()
}

// Server selection dialog positions (English version, Prguse.wil[256] background).
// Dialog is centered on screen. Button positions are relative to dialog origin.
const (
	srvDlgOX = loginOX + 192 // Dialog x offset (centered in 800-wide area)
	srvDlgOY = loginOY + 100 // Dialog y offset
)

// Server button positions relative to dialog origin (from Delphi FState.pas).
// English version: all use Prguse.wil[79], vertically stacked at x=65.
var srvButtonOffsets = []struct {
	dx, dy float32
}{
	{65, 100}, // Server 1
	{65, 145}, // Server 2
	{65, 190}, // Server 3
	{65, 235}, // Server 4
	{65, 280}, // Server 5
	{65, 325}, // Server 6
}

// Close button position relative to dialog origin.
const (
	srvCloseDX = float32(245)
	srvCloseDY = float32(31)
)

// NewSelectServerScene creates a new server selection scene.
func NewSelectServerScene(gl *engine.GLState, resources *engine.ResourceManager, text *engine.TextRenderer) *SelectServerScene {
	return &SelectServerScene{
		gl:        gl,
		resources: resources,
		text:      text,
		selected:  0,
	}
}

// SetServers sets the server list from SM_PASSOKSELECTSERVER body.
func (s *SelectServerScene) SetServers(servers []serverInfo) {
	s.servers = servers
	s.selected = 0
	if len(servers) > 0 {
		s.selected = 0
	}
	log.Logf(log.LevelInfo, "SelectServerScene", "Servers: %v", servers)
}

// SetSelectFunc sets the callback for server selection.
func (s *SelectServerScene) SetSelectFunc(fn func(serverName string)) {
	s.selectFunc = fn
}

// SetCloseFunc sets the callback for closing.
func (s *SelectServerScene) SetCloseFunc(fn func()) {
	s.closeFunc = fn
}

// Open is called when the scene becomes active.
func (s *SelectServerScene) Open() {
	log.Logf(log.LevelInfo, "SelectServerScene", "Opened")
}

// Close is called when the scene becomes inactive.
func (s *SelectServerScene) Close() {
	log.Logf(log.LevelInfo, "SelectServerScene", "Closed")
}

// Update updates the scene state.
func (s *SelectServerScene) Update(dt float64) {
}

// Render renders the server selection scene.
func (s *SelectServerScene) Render(gl *engine.GLState, proj [16]float32) {
	// Draw login background (same as login scene)
	if s.resources.ChrSel != nil {
		tex := s.resources.GetTexture(s.resources.ChrSel, 22)
		if tex != 0 {
			w, h := s.getChrSelSize(22)
			gl.DrawQuad(tex, loginOX, loginOY, float32(w), float32(h), proj)
		}
	}

	// Draw dialog background (Prguse.wil[256] for English version)
	if s.resources.Prguse != nil {
		dlgTex := s.resources.GetTexture(s.resources.Prguse, 256)
		if dlgTex != 0 {
			w, h := s.getPrguseSize(256)
			gl.DrawQuad(dlgTex, srvDlgOX, srvDlgOY, float32(w), float32(h), proj)
		}

		// Draw close button (Prguse.wil[83])
		closeTex := s.resources.GetTexture(s.resources.Prguse, 83)
		if closeTex != 0 {
			cw, ch := s.getPrguseSize(83)
			gl.DrawQuad(closeTex, srvDlgOX+srvCloseDX, srvDlgOY+srvCloseDY, float32(cw), float32(ch), proj)
		}

		// Draw server buttons (Prguse.wil[79])
		for i := 0; i < len(s.servers) && i < len(srvButtonOffsets); i++ {
			btnTex := s.resources.GetTexture(s.resources.Prguse, 79)
			if btnTex != 0 {
				bw, bh := s.getPrguseSize(79)
				bx := srvDlgOX + srvButtonOffsets[i].dx
				by := srvDlgOY + srvButtonOffsets[i].dy
				gl.DrawQuad(btnTex, bx, by, float32(bw), float32(bh), proj)
			}
		}
	}

	// Draw server names on buttons
	if s.text != nil {
		for i, srv := range s.servers {
			if i >= len(srvButtonOffsets) {
				break
			}
			bx := srvDlgOX + srvButtonOffsets[i].dx + 10
			by := srvDlgOY + srvButtonOffsets[i].dy + 5
			// Highlight selected
			if i == s.selected {
				s.text.DrawText(srv.Name, bx, by, 1.0, 1.0, 0.0, 1.0, proj)
			} else {
				s.text.DrawText(srv.Name, bx, by, 1.0, 1.0, 1.0, 1.0, proj)
			}
		}

		// Draw title
		titleX := srvDlgOX + 100
		titleY := srvDlgOY + 50
		s.text.DrawText("选择服务器", titleX, titleY, 1.0, 1.0, 1.0, 1.0, proj)
	}
}

// OnKey handles keyboard input.
func (s *SelectServerScene) OnKey(key int, action int) {
	if action != 1 {
		return
	}
	switch key {
	case keyEnter, keyKPEnter:
		log.Logf(log.LevelInfo, "SelectServerScene", "Enter pressed, confirming selection")
		s.confirmSelection()
	case keyEscape:
		log.Logf(log.LevelInfo, "SelectServerScene", "Escape pressed, closing")
		if s.closeFunc != nil {
			s.closeFunc()
		}
	}
}

// OnMouse handles mouse button input.
func (s *SelectServerScene) OnMouse(x, y float64, button int, action int) {
	fx, fy := float32(x), float32(y)
	log.Logf(log.LevelDebug, "SelectServerScene", "Mouse click at (%.0f, %.0f)", fx, fy)

	// Check close button
	closeRect := loginArea{
		srvDlgOX + srvCloseDX, srvDlgOY + srvCloseDY,
		20, 20, // approximate size
	}
	if hitTest(fx, fy, closeRect) {
		log.Logf(log.LevelInfo, "SelectServerScene", "Close button clicked")
		if s.closeFunc != nil {
			s.closeFunc()
		}
		return
	}

	// Check server buttons
	for i := range s.servers {
		if i >= len(srvButtonOffsets) {
			break
		}
		btnRect := loginArea{
			srvDlgOX + srvButtonOffsets[i].dx,
			srvDlgOY + srvButtonOffsets[i].dy,
			150, 35, // approximate button size
		}
		if hitTest(fx, fy, btnRect) {
			log.Logf(log.LevelInfo, "SelectServerScene", "Server button %d clicked: %s", i, s.servers[i].Name)
			s.selected = i
			s.confirmSelection()
			return
		}
	}
}

// OnScroll handles mouse scroll input.
func (s *SelectServerScene) OnScroll(x, y float64) {
}

// confirmSelection sends the selected server to the server.
func (s *SelectServerScene) confirmSelection() {
	if len(s.servers) == 0 {
		return
	}
	if s.selected < 0 || s.selected >= len(s.servers) {
		s.selected = 0
	}
	srv := s.servers[s.selected]
	log.Logf(log.LevelInfo, "SelectServerScene", "Selected server: %s", srv.Name)
	if s.selectFunc != nil {
		s.selectFunc(srv.Name)
	}
}

// getPrguseSize gets the size of a texture from Prguse.wil.
func (s *SelectServerScene) getPrguseSize(index int) (int, int) {
	if s.resources.Prguse == nil || index >= len(s.resources.Prguse.Images) {
		return 0, 0
	}
	img := s.resources.Prguse.Images[index]
	if img == nil {
		return 0, 0
	}
	return img.Width, img.Height
}

// getChrSelSize gets the size of a texture from ChrSel.wil.
func (s *SelectServerScene) getChrSelSize(index int) (int, int) {
	if s.resources.ChrSel == nil || index >= len(s.resources.ChrSel.Images) {
		return 0, 0
	}
	img := s.resources.ChrSel.Images[index]
	if img == nil {
		return 0, 0
	}
	return img.Width, img.Height
}

// parseServerList parses the server list from SM_PASSOKSELECTSERVER body.
// Body format: "name1/status1/name2/status2/..."
func parseServerList(body string) []serverInfo {
	var servers []serverInfo
	if body == "" {
		// Default server
		servers = append(servers, serverInfo{Name: "Server", Status: 1})
		return servers
	}
	parts := splitSlash(body)
	for i := 0; i+1 < len(parts); i += 2 {
		name := parts[i]
		status := 0
		fmt.Sscanf(parts[i+1], "%d", &status)
		if name != "" {
			servers = append(servers, serverInfo{Name: name, Status: status})
		}
	}
	if len(servers) == 0 {
		servers = append(servers, serverInfo{Name: "Server", Status: 1})
	}
	return servers
}

// splitSlash splits a string by '/'.
func splitSlash(s string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '/' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}
