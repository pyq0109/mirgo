package main

import (
	"os"

	"github.com/pyq0109/mirgo/internal/log"
)

type SafeZone struct {
	MapName string
	X, Y    int
	Radius  int
}

type safeZoneManager struct {
	zones []SafeZone
}

var globalSafeZones = &safeZoneManager{
	zones: []SafeZone{
		{MapName: "0", X: 289, Y: 618, Radius: 5},
	},
}

func LoadSafeZones(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Logf(log.LevelWarn, "SafeZone", "failed to load %s: %v, using defaults", path, err)
		return
	}

	var raw struct {
		StartPoints []struct {
			MapName string `json:"mapName"`
			X       int    `json:"x"`
			Y       int    `json:"y"`
			Range   int    `json:"range"`
		} `json:"startPoints"`
	}
	if err := parseJSONC(data, &raw); err != nil {
		log.Logf(log.LevelWarn, "SafeZone", "failed to parse %s: %v", path, err)
		return
	}

	var zones []SafeZone
	for _, sp := range raw.StartPoints {
		radius := sp.Range
		if radius <= 0 {
			radius = 5
		}
		zones = append(zones, SafeZone{
			MapName: sp.MapName,
			X:       sp.X,
			Y:       sp.Y,
			Radius:  radius,
		})
	}

	hasDefault := false
	for _, z := range zones {
		if z.MapName == "0" && z.X == 289 && z.Y == 618 {
			hasDefault = true
			break
		}
	}
	if !hasDefault {
		zones = append(zones, SafeZone{MapName: "0", X: 289, Y: 618, Radius: 5})
	}

	globalSafeZones.zones = zones
	log.Logf(log.LevelInfo, "SafeZone", "loaded %d safe zones from %s", len(zones), path)
}

func CheckSafeZone(mapName string, x, y int) bool {
	for _, zone := range globalSafeZones.zones {
		if zone.MapName != mapName {
			continue
		}
		dx := x - zone.X
		dy := y - zone.Y
		if dx < 0 {
			dx = -dx
		}
		if dy < 0 {
			dy = -dy
		}
		if dx <= zone.Radius && dy <= zone.Radius {
			return true
		}
	}
	return false
}

func GetSafeZonePoint() (mapName string, x, y int) {
	if len(globalSafeZones.zones) > 0 {
		z := globalSafeZones.zones[0]
		return z.MapName, z.X, z.Y
	}
	return "0", 289, 618
}
