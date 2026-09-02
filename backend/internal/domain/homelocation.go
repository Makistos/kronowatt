package domain

import "time"

// HomeLocation is the household's coordinates and the FMI station chosen
// for weather observations (spec §4.1) — a singleton, not a table of many
// locations.
type HomeLocation struct {
	Latitude      float64
	Longitude     float64
	StationFMISID string
	StationName   string
	UpdatedAt     time.Time
}
