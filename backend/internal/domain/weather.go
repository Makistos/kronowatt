package domain

import (
	"encoding/json"
	"time"
)

// WeatherObservation mirrors the weather_observation table (spec §4.2).
// Pointer fields are optional — a missing parameter from the source must
// not fail the whole observation.
type WeatherObservation struct {
	Time             time.Time
	StationFMISID    string
	AirTemperature   *float64
	RelativeHumidity *float64
	DewPoint         *float64
	AirPressure      *float64
	WindSpeed        *float64
	WindDirection    *float64
	WindGust         *float64
	Precipitation    *float64
	CloudCover       *float64
	Visibility       *float64
	RawPayload       json.RawMessage
	RetrievedAt      time.Time
}
