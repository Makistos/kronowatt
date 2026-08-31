// Package api is the HTTP layer — it depends on storage repositories, never
// on collector internals (spec §2.5), and is the only package that knows
// about JSON wire shapes.
package api

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"kronowatt/backend/internal/storage"
)

func NewRouter(pool *pgxpool.Pool, diskCheckPath string) http.Handler {
	weatherRepo := storage.NewWeatherRepository(pool)
	collectorRepo := storage.NewCollectorRepository(pool)
	electricityRepo := storage.NewElectricityRepository(pool)
	spotPriceRepo := storage.NewSpotPriceRepository(pool)
	evRepo := storage.NewEVRepository(pool)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthHandler(collectorRepo, diskCheckPath))
	mux.HandleFunc("GET /api/meta/years", yearsHandler(electricityRepo))
	mux.HandleFunc("GET /api/weather/observations", weatherObservationsHandler(weatherRepo))
	mux.HandleFunc("GET /api/electricity/measurements", electricityMeasurementsHandler(electricityRepo))
	mux.HandleFunc("GET /api/spot-prices", spotPricesHandler(spotPriceRepo))
	mux.HandleFunc("GET /api/ev/sessions", evSessionsHandler(evRepo))

	return withCORS(mux)
}

// withCORS allows any origin for GET requests. The frontend and backend
// run as separate services on separate ports even in production (spec §9
// deployment split), so cross-origin requests are the normal case, not an
// edge case — and this is a LAN-only app (spec §1), so a permissive origin
// is an acceptable tradeoff for not having to keep an allowlist in sync
// with whatever port/host the frontend happens to be served from.
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
