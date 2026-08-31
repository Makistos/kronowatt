package api

import (
	"net/http"
	"time"

	"kronowatt/backend/internal/domain"
	"kronowatt/backend/internal/storage"
)

type evSessionDTO struct {
	StartTime      string   `json:"start_time"`
	EndTime        *string  `json:"end_time,omitempty"`
	EnergyKWh      *float64 `json:"energy_kwh,omitempty"`
	AveragePowerKW *float64 `json:"average_power_kw,omitempty"`
	MaximumPowerKW *float64 `json:"maximum_power_kw,omitempty"`
	Source         string   `json:"source"`
}

func evSessionsHandler(repo *storage.EVRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start, end, err := parseDateRange(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		rows, err := repo.ListSessions(r.Context(), start, end)
		if err != nil {
			http.Error(w, "failed to load EV sessions", http.StatusInternalServerError)
			return
		}

		dtos := make([]evSessionDTO, len(rows))
		for i, s := range rows {
			dtos[i] = toEVSessionDTO(s)
		}
		writeJSON(w, http.StatusOK, dtos)
	}
}

func toEVSessionDTO(s domain.EVChargingSession) evSessionDTO {
	dto := evSessionDTO{
		StartTime:      s.StartTime.UTC().Format(time.RFC3339),
		EnergyKWh:      s.EnergyKWh,
		AveragePowerKW: s.AveragePowerKW,
		MaximumPowerKW: s.MaximumPowerKW,
		Source:         s.Source,
	}
	if s.EndTime != nil {
		end := s.EndTime.UTC().Format(time.RFC3339)
		dto.EndTime = &end
	}
	return dto
}
