package mapformat

import (
	"os"
	"testing"
)

func TestParseHeader(t *testing.T) {
	// Test with a known map file (relative to project root)
	path := "../../asset/server/Map/0102.map"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("map file not found:", path)
	}

	m, err := Parse(path)
	if err != nil {
		t.Fatal("parse failed:", err)
	}

	if m.Width == 0 || m.Height == 0 {
		t.Fatalf("invalid dimensions: %dx%d", m.Width, m.Height)
	}

	if len(m.Cells) != m.Width*m.Height {
		t.Fatalf("cell count mismatch: got %d, want %d", len(m.Cells), m.Width*m.Height)
	}

	t.Logf("Map: %dx%d, cells=%d", m.Width, m.Height, len(m.Cells))
}

func TestParseColumnMajor(t *testing.T) {
	path := "../../asset/server/Map/0102.map"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("map file not found:", path)
	}

	m, err := Parse(path)
	if err != nil {
		t.Fatal("parse failed:", err)
	}

	// Verify At() works for all cells
	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			cell := m.At(x, y)
			if cell == nil {
				t.Fatalf("nil cell at (%d,%d)", x, y)
			}
		}
	}
}

func TestIsCollision(t *testing.T) {
	path := "../../asset/server/Map/0102.map"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("map file not found:", path)
	}

	m, err := Parse(path)
	if err != nil {
		t.Fatal("parse failed:", err)
	}

	// Just verify it doesn't panic
	collisionCount := 0
	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			if m.IsCollision(x, y) {
				collisionCount++
			}
		}
	}
	t.Logf("Collision cells: %d/%d", collisionCount, len(m.Cells))
}
