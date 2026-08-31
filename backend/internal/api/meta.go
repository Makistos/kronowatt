package api

import (
	"net/http"

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
