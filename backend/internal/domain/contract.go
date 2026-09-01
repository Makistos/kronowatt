package domain

import "time"

type PricingModel string

const (
	PricingModelFixed PricingModel = "fixed"
	PricingModelSpot  PricingModel = "spot"
)

// Contract is one household pricing configuration, effective from
// ValidFrom until superseded by the next contract with a later ValidFrom
// (spec §2.4/§6 — "effective price determined by timestamp", historical
// contract data immutable). Unlike spot_price (which stores the raw
// €/MWh market unit), every price field here is stored in the unit the
// settings UI collects it in — c/kWh for the two per-energy rates, EUR
// for the flat recurring charges — since this is manually entered data
// with no external wire format to preserve.
type Contract struct {
	ID           int64
	PricingModel PricingModel
	ValidFrom    time.Time
	Period       ContractPeriod
}

// ContractPeriod holds the actual numbers. EnergyPrice and SpotMargin are
// mutually exclusive per PricingModel: a fixed contract sets EnergyPrice
// (absolute c/kWh) and leaves SpotMargin nil; a spot contract sets
// SpotMargin (c/kWh added on top of the market price, e.g. 0.49) and
// leaves EnergyPrice nil.
type ContractPeriod struct {
	EnergyPrice    *float64 // c/kWh — fixed contracts only
	SpotMargin     *float64 // c/kWh — spot contracts only, added on top of the spot price
	TransferPrice  float64  // c/kWh
	MonthlyFee     float64  // EUR
	ElectricityTax float64  // EUR
	TransferTax    float64  // EUR
}
