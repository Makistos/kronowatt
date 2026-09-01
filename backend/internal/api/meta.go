package api

import (
	"net/http"
	"time"

	"kronowatt/backend/internal/storage"
)

type yearsResponse struct {
	Years []int `json:"years"`
}

// yearsHandler lets the frontend discover which years have data instead of
// hardcoding a year list (it previously read this from the fake-data
// manifest.json; this is the real-backend equivalent).
func yearsHandler(repo *storage.ElectricityRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		years, err := repo.DistinctYears(r.Context())
		if err != nil {
			http.Error(w, "failed to load years", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, yearsResponse{Years: years})
	}
}

type dateRangeResponse struct {
	Min *string `json:"min"`
	Max *string `json:"max"`
}

// rangeHandler lets the frontend default date/month/year pickers to the
// actual latest data rather than assuming a full calendar year exists — in
// early production there's a partial first year, not a complete Jan-Dec
// span. Min/Max are null (not omitted) when the table is empty, so the
// frontend can tell "no data yet" apart from a request error.
func rangeHandler(repo *storage.ElectricityRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		min, max, err := repo.DateRange(r.Context())
		if err != nil {
			http.Error(w, "failed to load date range", http.StatusInternalServerError)
			return
		}

		var resp dateRangeResponse
		if min != nil {
			s := min.UTC().Format(time.RFC3339)
			resp.Min = &s
		}
		if max != nil {
			s := max.UTC().Format(time.RFC3339)
			resp.Max = &s
		}
		writeJSON(w, http.StatusOK, resp)
	}
}
