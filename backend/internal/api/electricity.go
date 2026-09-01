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
	// PowerKW/PhasesKW are p[0]/p[1:], not stored as separate columns.
	// Deliberately no "energy_kwh" field here: converting power to energy
	// needs the sample interval, which this API doesn't track (real Cozify
	// samples land every ~10s; the seeded fake data is 15-min) — that
	// conversion belongs wherever the caller knows what resolution it
	// asked for, not baked into this DTO as a hardcoded assumption.
	PowerKW  *float64  `json:"power_kw,omitempty"`
	PhasesKW []float64 `json:"phases_kw,omitempty"`
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
	}
	if len(m.P) > 1 {
		dto.PhasesKW = m.P[1:]
	}
	return dto
}
