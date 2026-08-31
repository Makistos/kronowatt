package domain

import "time"

// SpotPrice mirrors the spot_price table (spec §5) — 15-minute intervals,
// not points.
type SpotPrice struct {
	IntervalStart time.Time
	IntervalEnd   time.Time
	Price         float64
	Currency      string
	Unit          string
	Source        string
	RetrievedAt   time.Time
}
