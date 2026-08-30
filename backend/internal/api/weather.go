package api

import (
	"net/http"
	"time"

	"kronowatt/backend/internal/domain"
	"kronowatt/backend/internal/storage"
)

type weatherObservationDTO struct {
	Time             string   `json:"time"`
	StationFMISID    string   `json:"station_fmisid"`
	AirTemperature   *float64 `json:"air_temperature,omitempty"`
	RelativeHumidity *float64 `json:"relative_humidity,omitempty"`
	DewPoint         *float64 `json:"dew_point,omitempty"`
	AirPressure      *float64 `json:"air_pressure,omitempty"`
	WindSpeed        *float64 `json:"wind_speed,omitempty"`
	WindDirection    *float64 `json:"wind_direction,omitempty"`
	WindGust         *float64 `json:"wind_gust,omitempty"`
	Precipitation    *float64 `json:"precipitation,omitempty"`
	CloudCover       *float64 `json:"cloud_cover,omitempty"`
	Visibility       *float64 `json:"visibility,omitempty"`
}

func weatherObservationsHandler(repo *storage.WeatherRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start, end, err := parseDateRange(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		obs, err := repo.ListObservations(r.Context(), start, end)
		if err != nil {
			http.Error(w, "failed to load observations", http.StatusInternalServerError)
			return
		}

		dtos := make([]weatherObservationDTO, len(obs))
		for i, o := range obs {
			dtos[i] = toWeatherObservationDTO(o)
		}
		writeJSON(w, http.StatusOK, dtos)
	}
}

func toWeatherObservationDTO(o domain.WeatherObservation) weatherObservationDTO {
	return weatherObservationDTO{
		Time:             o.Time.UTC().Format(time.RFC3339),
		StationFMISID:    o.StationFMISID,
		AirTemperature:   o.AirTemperature,
		RelativeHumidity: o.RelativeHumidity,
		DewPoint:         o.DewPoint,
		AirPressure:      o.AirPressure,
		WindSpeed:        o.WindSpeed,
		WindDirection:    o.WindDirection,
		WindGust:         o.WindGust,
		Precipitation:    o.Precipitation,
		CloudCover:       o.CloudCover,
		Visibility:       o.Visibility,
	}
}
