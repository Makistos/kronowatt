package domain

import "time"

// EVChargingSession mirrors ev_charging_session (spec §7.3) — field list is
// provisional pending verification against ha-defa-power's actual schema.
type EVChargingSession struct {
	StartTime      time.Time
	EndTime        *time.Time
	EnergyKWh      *float64
	AveragePowerKW *float64
	MaximumPowerKW *float64
	Source         string
}
