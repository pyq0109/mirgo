package main

import (
	"github.com/pyq0109/mirgo/internal/log"
)

const (
	doorCloseDelay = 5000 // 5 seconds in milliseconds
)

// ProcessDoors processes door state changes.
func ProcessDoors(envir *Environment, currentTick int64) {
	for i := range envir.Doors {
		door := &envir.Doors[i]
		if door.State == 1 && currentTick-door.OpenTick > doorCloseDelay {
			// Close the door
			door.State = 0
			log.Logf(log.LevelDebug, "Doors", "Door closed at (%d,%d)", door.X, door.Y)

			// Update map cell
			idx := door.Y*envir.Width + door.X
			if idx >= 0 && idx < len(envir.Cells) {
				// Remove door offset from front image
				// TODO: Update the map cell to reflect door state
			}
		}
	}
}
