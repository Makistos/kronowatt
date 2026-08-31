package api

import (
	"net/http"
	"time"

	"kronowatt/backend/internal/domain"
	"kronowatt/backend/internal/storage"
)

type electricityMeasurementDTO struct {
	Time   string   `json:"time"`
	Source string   `json:"source"`
	IC     *float64 `json:"ic,omitempty"`
	EC     *float64 `json:"ec,omitempty"`
	// PowerKW/EnergyKWh are derived from p[0], not stored directly. Energy
	// is only meaningful as-is for hourly-bucketed rows (the seeded fake
	// data): real Cozify samples land every ~10s, so deriving energy for
	// those will need proper time-integration, not this shortcut — revisit
	// once real device data exists (spec §3.2 field units are still
	// unconfirmed too).
	PowerKW   *float64 `json:"power_kw,omitempty"`
	EnergyKWh *float64 `json:"energy_kwh,omitempty"`
}

func electricityMeasurementsHandler(repo *storage.ElectricityRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start, end, err := parseDateRange(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		rows, err := repo.ListMeasurements(r.Context(), start, end)
		if err != nil {
			http.Error(w, "failed to load measurements", http.StatusInternalServerError)
			return
		}

		dtos := make([]electricityMeasurementDTO, len(rows))
		for i, m := range rows {
			dtos[i] = toElectricityMeasurementDTO(m)
		}
		writeJSON(w, http.StatusOK, dtos)
	}
}

func toElectricityMeasurementDTO(m domain.ElectricityMeasurement) electricityMeasurementDTO {
	dto := electricityMeasurementDTO{
		Time:   m.Time.UTC().Format(time.RFC3339),
		Source: m.Source,
		IC:     m.IC,
		EC:     m.EC,
	}
	if len(m.P) > 0 {
		dto.PowerKW = &m.P[0]
		dto.EnergyKWh = &m.P[0]
	}
	return dto
}
