package main

import (
	"testing"

	"github.com/pyq0109/mirgo/internal/protocol"
)

// ============================================================================
// Message Parsing Tests
// ============================================================================

func TestParseFirstServer(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		expected string
	}{
		{"empty", "", "Server"},
		{"single server", "Server1/1", "Server1"},
		{"multiple servers", "Server1/1/Server2/2/Server3/3", "Server1"},
		{"name only", "MyServer", "MyServer"},
		{"empty name", "/1", "Server"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseFirstServer(tt.body)
			if result != tt.expected {
				t.Errorf("parseFirstServer(%q) = %q, want %q", tt.body, result, tt.expected)
			}
		})
	}
}

func TestParseAddrPortCert(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantAddr string
		wantCert int
		wantErr  bool
	}{
		{"valid", "localhost/7000/12345", "localhost:7000", 12345, false},
		{"valid IP", "192.168.1.1/7100/99999", "192.168.1.1:7100", 99999, false},
		{"empty", "", "", 0, true},
		{"too few parts", "localhost/7000", "", 0, true},
		{"invalid cert", "localhost/7000/abc", "", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, cert, err := parseAddrPortCert(tt.body)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got addr=%s cert=%d", addr, cert)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if addr != tt.wantAddr {
				t.Errorf("addr = %q, want %q", addr, tt.wantAddr)
			}
			if cert != tt.wantCert {
				t.Errorf("cert = %d, want %d", cert, tt.wantCert)
			}
		})
	}
}

func TestParseAddrPort(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantAddr string
		wantErr  bool
	}{
		{"valid", "localhost/7000", "localhost:7000", false},
		{"valid IP", "192.168.1.1/7100", "192.168.1.1:7100", false},
		{"empty", "", "", true},
		{"too few parts", "localhost", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, err := parseAddrPort(tt.body)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got addr=%s", addr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if addr != tt.wantAddr {
				t.Errorf("addr = %q, want %q", addr, tt.wantAddr)
			}
		})
	}
}

func TestParseQueryChrBody(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantChars   int
		wantSelect  int
		wantFirst   parsedChar
	}{
		{
			name:       "empty",
			body:       "",
			wantChars:  0,
			wantSelect: -1,
		},
		{
			name:       "single character selected",
			body:       "*Warrior/0/0/10/1",
			wantChars:  1,
			wantSelect: 0,
			wantFirst:  parsedChar{Name: "Warrior", Job: 0, Hair: 0, Level: 10, Sex: 1},
		},
		{
			name:       "two characters first selected",
			body:       "*Warrior/0/0/10/1/Wizard/1/0/5/0",
			wantChars:  2,
			wantSelect: 0,
			wantFirst:  parsedChar{Name: "Warrior", Job: 0, Hair: 0, Level: 10, Sex: 1},
		},
		{
			name:       "two characters second selected",
			body:       "Warrior/0/0/10/1/*Wizard/1/0/5/0",
			wantChars:  2,
			wantSelect: 1,
			wantFirst:  parsedChar{Name: "Warrior", Job: 0, Hair: 0, Level: 10, Sex: 1},
		},
		{
			name:       "no selected marker",
			body:       "Warrior/0/0/10/1/Wizard/1/0/5/0",
			wantChars:  2,
			wantSelect: -1,
			wantFirst:  parsedChar{Name: "Warrior", Job: 0, Hair: 0, Level: 10, Sex: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chars, selectedIdx := parseQueryChrBody(tt.body)
			if len(chars) != tt.wantChars {
				t.Fatalf("len(chars) = %d, want %d", len(chars), tt.wantChars)
			}
			if selectedIdx != tt.wantSelect {
				t.Errorf("selectedIdx = %d, want %d", selectedIdx, tt.wantSelect)
			}
			if tt.wantChars > 0 {
				c := chars[0]
				if c.Name != tt.wantFirst.Name {
					t.Errorf("char[0].Name = %q, want %q", c.Name, tt.wantFirst.Name)
				}
				if c.Job != tt.wantFirst.Job {
					t.Errorf("char[0].Job = %d, want %d", c.Job, tt.wantFirst.Job)
				}
				if c.Level != tt.wantFirst.Level {
					t.Errorf("char[0].Level = %d, want %d", c.Level, tt.wantFirst.Level)
				}
				if c.Sex != tt.wantFirst.Sex {
					t.Errorf("char[0].Sex = %d, want %d", c.Sex, tt.wantFirst.Sex)
				}
			}
		})
	}
}

// ============================================================================
// Protocol Message Format Tests
// ============================================================================

func TestLoginMessageFormat(t *testing.T) {
	// Verify CMIDPassword message format
	msg := protocol.MakeDefaultMsg(protocol.CMIDPassword, 0, 0, 0, 0)
	encoded := protocol.EncodeMessage(msg)
	decoded := protocol.DecodeMessage(encoded)

	if decoded.Ident != protocol.CMIDPassword {
		t.Errorf("Ident = %d, want %d", decoded.Ident, protocol.CMIDPassword)
	}
	if decoded.Recog != 0 {
		t.Errorf("Recog = %d, want 0", decoded.Recog)
	}
}

func TestCredentialBodyFormat(t *testing.T) {
	// Verify "username/password" encoding round trip
	body := "testuser/testpass"
	encoded := protocol.EncodeString(body)
	decoded := protocol.DecodeString(encoded)

	if decoded != body {
		t.Errorf("credential body = %q, want %q", decoded, body)
	}
}

func TestSMSelectServerOKFormat(t *testing.T) {
	// Verify "addr/port/cert" body format
	body := "localhost/7000/12345"
	encoded := protocol.EncodeString(body)
	decoded := protocol.DecodeString(encoded)

	if decoded != body {
		t.Errorf("SMSelectServerOK body = %q, want %q", decoded, body)
	}
}

func TestSMQueryChrTextFormat(t *testing.T) {
	// Verify "*name/job/hair/level/sex" text format
	body := "*Warrior/0/0/10/1/Wizard/1/0/5/0"
	chars, selectedIdx := parseQueryChrBody(body)

	if len(chars) != 2 {
		t.Fatalf("len(chars) = %d, want 2", len(chars))
	}
	if selectedIdx != 0 {
		t.Errorf("selectedIdx = %d, want 0", selectedIdx)
	}
	if chars[0].Name != "Warrior" || chars[0].Job != 0 || chars[0].Level != 10 || chars[0].Sex != 1 {
		t.Errorf("chars[0] = %+v, want Warrior/0/0/10/1", chars[0])
	}
	if chars[1].Name != "Wizard" || chars[1].Job != 1 || chars[1].Level != 5 || chars[1].Sex != 0 {
		t.Errorf("chars[1] = %+v, want Wizard/1/0/5/0", chars[1])
	}
}

func TestSMStartPlayFormat(t *testing.T) {
	// Verify "addr/port" body format
	body := "localhost/7000"
	addr, err := parseAddrPort(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if addr != "localhost:7000" {
		t.Errorf("addr = %q, want %q", addr, "localhost:7000")
	}
}

func TestRunLoginFormat(t *testing.T) {
	// Verify **loginID/charName/cert/version/code format
	loginID := "testuser"
	charName := "Warrior"

	s := "**" + loginID + "/" + charName + "/" + "12345" + "/" + "120040918" + "/9"

	// Verify encoding round trip
	encoded := protocol.EncodeString(s)
	decoded := protocol.DecodeString(encoded)

	if decoded != s {
		t.Errorf("run login = %q, want %q", decoded, s)
	}

	// Verify parsing
	if decoded[:2] != "**" {
		t.Errorf("prefix = %q, want **", decoded[:2])
	}

}

// ============================================================================
// Server Frame Format Test
// ============================================================================

func TestServerFrameRoundTrip(t *testing.T) {
	// Server sends: #<payload>!
	// Client expects: #<payload>! (no digit prefix)
	msg := protocol.MakeDefaultMsg(protocol.SMPasswdFail, -1, 0, 0, 0)
	encoded := protocol.EncodeMessage(msg)
	frame := protocol.FormatServerFrame(encoded)

	if frame[0] != '#' || frame[len(frame)-1] != '!' {
		t.Errorf("frame format = %q, expected #...!", frame)
	}

	// Simulate client parsing
	payload := frame[1 : len(frame)-1]
	if len(payload) >= protocol.DefBlockSize {
		decoded := protocol.DecodeMessage(payload[:protocol.DefBlockSize])
		if decoded.Ident != protocol.SMPasswdFail {
			t.Errorf("Ident = %d, want %d", decoded.Ident, protocol.SMPasswdFail)
		}
		if decoded.Recog != -1 {
			t.Errorf("Recog = %d, want -1", decoded.Recog)
		}
	}
}

func TestClientFrameWithCode(t *testing.T) {
	// Client sends: #<code><payload>!
	msg := protocol.MakeDefaultMsg(protocol.CMIDPassword, 0, 0, 0, 0)
	encoded := protocol.EncodeMessage(msg)
	body := protocol.EncodeString("user/pass")
	code := byte(1)
	frame := protocol.FormatClientFrame(encoded+body, &code)

	if frame[0] != '#' {
		t.Errorf("frame[0] = %c, want #", frame[0])
	}
	if frame[1] != '1' {
		t.Errorf("frame[1] = %c, want 1", frame[1])
	}
	if frame[len(frame)-1] != '!' {
		t.Errorf("frame[-1] = %c, want !", frame[len(frame)-1])
	}
}
