package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"kronowatt/backend/internal/collectors/fmi"
	"kronowatt/backend/internal/domain"
	"kronowatt/backend/internal/storage"
)

// homeLocationDTO's fields match exactly what the settings UI collects and
// displays (spec §4.1's "User location... used for forecasts", extended
// here to also drive which FMI station the observation collector polls).
type homeLocationDTO struct {
	Latitude      float64 `json:"latitude"`
	Longitude     float64 `json:"longitude"`
	StationFMISID string  `json:"station_fmisid"`
	StationName   string  `json:"station_name"`
}

func toHomeLocationDTO(l domain.HomeLocation) homeLocationDTO {
	return homeLocationDTO{
		Latitude:      l.Latitude,
		Longitude:     l.Longitude,
		StationFMISID: l.StationFMISID,
		StationName:   l.StationName,
	}
}

// homeLocationHandler serves GET (current location, or JSON null if never
// configured — same "null, not 404, so the frontend can tell 'not set yet'
// apart from an error" pattern as meta.go's rangeHandler) and PUT (save).
func homeLocationHandler(repo *storage.HomeLocationRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			loc, ok, err := repo.Get(r.Context())
			if err != nil {
				http.Error(w, "failed to load home location", http.StatusInternalServerError)
				return
			}
			if !ok {
				writeJSON(w, http.StatusOK, nil)
				return
			}
			writeJSON(w, http.StatusOK, toHomeLocationDTO(loc))

		case http.MethodPut:
			var req homeLocationDTO
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid request body", http.StatusBadRequest)
				return
			}
			if req.StationFMISID == "" || req.StationName == "" {
				http.Error(w, "station_fmisid and station_name are required", http.StatusBadRequest)
				return
			}

			saved, err := repo.Upsert(r.Context(), domain.HomeLocation{
				Latitude:      req.Latitude,
				Longitude:     req.Longitude,
				StationFMISID: req.StationFMISID,
				StationName:   req.StationName,
			})
			if err != nil {
				http.Error(w, "failed to save home location", http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusOK, toHomeLocationDTO(saved))

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

type nearestStationDTO struct {
	FMISID     string  `json:"fmisid"`
	Name       string  `json:"name"`
	Latitude   float64 `json:"latitude"`
	Longitude  float64 `json:"longitude"`
	DistanceKM float64 `json:"distance_km"`
}

// nearestStationsHandler is a live lookup against FMI's own APIs, not
// stored data — there's nothing in `storage` to serve this from, since
// "which stations exist near here" isn't something this app collects or
// persists. Calling into internal/collectors/fmi's exported
// FindNearbyStations directly is a deliberate, narrow exception to spec
// §2.5's "api/ depends on storage, not collector internals": it's a public
// function already returning clean Station/StationDistance types (no raw
// FMI XML reaches this handler), not a reach into unexported collector
// state — see CLAUDE.md "Home location and nearest-station search".
func nearestStationsHandler(client *http.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		lat, err := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
		if err != nil {
			http.Error(w, "invalid or missing lat", http.StatusBadRequest)
			return
		}
		lon, err := strconv.ParseFloat(r.URL.Query().Get("lon"), 64)
		if err != nil {
			http.Error(w, "invalid or missing lon", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()

		results, err := fmi.FindNearbyStations(ctx, client, lat, lon, 3)
		if err != nil {
			http.Error(w, "failed to search FMI stations: "+err.Error(), http.StatusBadGateway)
			return
		}

		dtos := make([]nearestStationDTO, len(results))
		for i, r := range results {
			dtos[i] = nearestStationDTO{
				FMISID:     r.FMISID,
				Name:       r.Name,
				Latitude:   r.Latitude,
				Longitude:  r.Longitude,
				DistanceKM: r.DistanceKM,
			}
		}
		writeJSON(w, http.StatusOK, dtos)
	}
}
