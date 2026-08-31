package domain

import (
	"encoding/json"
	"time"
)

// ElectricityMeasurement mirrors the electricity_measurement table (spec
// §3.2). Field names match the Cozify HAN wire format; most are optional
// since the real device's exact field set is still unconfirmed.
type ElectricityMeasurement struct {
	Time       time.Time
	Source     string
	IC         *float64 // cumulative imported energy
	EC         *float64 // cumulative exported energy
	RIC        *float64
	REC        *float64
	P          []float64 // instantaneous active power (total + phases)
	PI         []float64
	PE         []float64
	R          []float64
	RI         []float64
	RE         []float64
	U          []float64
	I          []float64
	RawPayload json.RawMessage
	InsertedAt time.Time
}
