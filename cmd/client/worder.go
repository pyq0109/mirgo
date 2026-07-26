package main

func getWordOrder(sex int, frame int) int {
	if frame < 0 || frame >= 600 {
		return 0
	}
	dirFrame := frame % 8
	switch {
	case frame < 64: // Stand
		if dirFrame >= 2 && dirFrame <= 3 {
			return 1
		}
		return 0
	case frame < 128: // Walk
		if dirFrame >= 1 && dirFrame <= 4 {
			return 1
		}
		return 0
	case frame < 192: // Run
		if dirFrame >= 1 && dirFrame <= 4 {
			return 1
		}
		return 0
	case frame < 200: // RushLeft/RushRight/WarMode
		return 0
	case frame < 264: // Hit
		if dirFrame >= 2 && dirFrame <= 5 {
			return 1
		}
		return 0
	case frame < 328: // HeavyHit
		if dirFrame >= 2 && dirFrame <= 5 {
			return 1
		}
		return 0
	case frame < 392: // BigHit
		if dirFrame >= 3 && dirFrame <= 6 {
			return 1
		}
		return 0
	case frame < 456: // Spell
		return 0
	case frame < 472: // Sitdown
		return 0
	case frame < 536: // Struck
		return 0
	default: // Die
		return 0
	}
}
