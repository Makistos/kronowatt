package api

import (
	"net/http"
	"time"

	"kronowatt/backend/internal/domain"
	"kronowatt/backend/internal/storage"
)

type spotPriceDTO struct {
	IntervalStart string  `json:"interval_start"`
	IntervalEnd   string  `json:"interval_end"`
	Price         float64 `json:"price"`
	Currency      string  `json:"currency"`
	Unit          string  `json:"unit"`
	Source        string  `json:"source"`
}

func spotPricesHandler(repo *storage.SpotPriceRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start, end, err := parseDateRange(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		rows, err := repo.ListPrices(r.Context(), start, end)
		if err != nil {
			http.Error(w, "failed to load spot prices", http.StatusInternalServerError)
			return
		}

		dtos := make([]spotPriceDTO, len(rows))
		for i, p := range rows {
			dtos[i] = toSpotPriceDTO(p)
		}
		writeJSON(w, http.StatusOK, dtos)
	}
}

func toSpotPriceDTO(p domain.SpotPrice) spotPriceDTO {
	return spotPriceDTO{
		IntervalStart: p.IntervalStart.UTC().Format(time.RFC3339),
		IntervalEnd:   p.IntervalEnd.UTC().Format(time.RFC3339),
		Price:         p.Price,
		Currency:      p.Currency,
		Unit:          p.Unit,
		Source:        p.Source,
	}
}
