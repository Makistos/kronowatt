// Package fmi collects weather observations from FMI's open data WFS API
// (spec §4). No auth required; it's a public documented API, unlike
// Cozify (no device available yet) or Defa (unofficial/reverse-engineered).
package fmi

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"time"

	"kronowatt/backend/internal/domain"
)

const baseURL = "https://opendata.fmi.fi/wfs"

// observationBuilder maps FMI's "simple" stored-query parameter codes to
// the weather_observation columns they fill. Confirmed against a live
// response for FMISID 101786 (Oulu lentoasema) — see the field list in
// spec §4.2. Anything the API returns that isn't handled in set() below
// still gets preserved in RawPayload, just not broken out into its own
// column.
type observationBuilder struct {
	airTemperature   *float64
	relativeHumidity *float64
	dewPoint         *float64
	airPressure      *float64
	windSpeed        *float64
	windDirection    *float64
	windGust         *float64
	precipitation    *float64
	cloudCover       *float64
	visibility       *float64
}

func (b *observationBuilder) set(param string, value float64) {
	v := value
	switch param {
	case "t2m":
		b.airTemperature = &v
	case "rh":
		b.relativeHumidity = &v
	case "td":
		b.dewPoint = &v
	case "p_sea":
		b.airPressure = &v
	case "ws_10min":
		b.windSpeed = &v
	case "wd_10min":
		b.windDirection = &v
	case "wg_10min":
		b.windGust = &v
	case "r_1h":
		b.precipitation = &v
	case "n_man":
		b.cloudCover = &v
	case "vis":
		b.visibility = &v
	}
}

// --- FMI WFS "simple" stored query response shape. encoding/xml matches
// struct tags against element local names, ignoring namespace prefixes
// (wfs:, BsWfs:, gml:) when the tag doesn't specify a namespace — verified
// against a live response rather than assumed. ---

type featureCollection struct {
	Members []member `xml:"member"`
}

type member struct {
	Element bsWfsElement `xml:"BsWfsElement"`
}

type bsWfsElement struct {
	Time           string  `xml:"Time"`
	ParameterName  string  `xml:"ParameterName"`
	ParameterValue float64 `xml:"ParameterValue"`
}

// FetchObservations queries FMI for FMISID's observations. If since is
// non-zero it's passed as the query window start; otherwise FMI's default
// window applies (~12h at 10min resolution for this station, verified
// live — enough natural backfill margin for normal poll intervals without
// needing an explicit starttime).
func FetchObservations(ctx context.Context, client *http.Client, fmisid string, since time.Time) ([]domain.WeatherObservation, error) {
	q := url.Values{}
	q.Set("service", "WFS")
	q.Set("version", "2.0.0")
	q.Set("request", "getFeature")
	q.Set("storedquery_id", "fmi::observations::weather::simple")
	q.Set("fmisid", fmisid)
	if !since.IsZero() {
		q.Set("starttime", since.UTC().Format(time.RFC3339))
	}

	reqURL := baseURL + "?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fmi returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	return parseObservations(body, fmisid)
}

func parseObservations(body []byte, fmisid string) ([]domain.WeatherObservation, error) {
	var fc featureCollection
	if err := xml.Unmarshal(body, &fc); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	byTime := map[string]*observationBuilder{}
	rawByTime := map[string]map[string]float64{}
	for _, m := range fc.Members {
		e := m.Element
		if _, ok := byTime[e.Time]; !ok {
			byTime[e.Time] = &observationBuilder{}
			rawByTime[e.Time] = map[string]float64{}
		}
		// FMI encodes "no observation for this parameter at this time" as
		// literal NaN, not by omitting the element. json.Marshal errors on
		// NaN, and it isn't a real reading either way, so drop it — same
		// outcome as if the parameter were simply missing from the response.
		if math.IsNaN(e.ParameterValue) {
			continue
		}
		byTime[e.Time].set(e.ParameterName, e.ParameterValue)
		rawByTime[e.Time][e.ParameterName] = e.ParameterValue
	}

	times := make([]string, 0, len(byTime))
	for t := range byTime {
		times = append(times, t)
	}
	sort.Strings(times)

	retrievedAt := time.Now().UTC()
	observations := make([]domain.WeatherObservation, 0, len(times))
	for _, t := range times {
		ts, err := time.Parse(time.RFC3339, t)
		if err != nil {
			return nil, fmt.Errorf("parse observation time %q: %w", t, err)
		}

		raw, err := json.Marshal(rawByTime[t])
		if err != nil {
			return nil, fmt.Errorf("marshal raw payload: %w", err)
		}

		b := byTime[t]
		observations = append(observations, domain.WeatherObservation{
			Time:             ts,
			StationFMISID:    fmisid,
			AirTemperature:   b.airTemperature,
			RelativeHumidity: b.relativeHumidity,
			DewPoint:         b.dewPoint,
			AirPressure:      b.airPressure,
			WindSpeed:        b.windSpeed,
			WindDirection:    b.windDirection,
			WindGust:         b.windGust,
			Precipitation:    b.precipitation,
			CloudCover:       b.cloudCover,
			Visibility:       b.visibility,
			RawPayload:       raw,
			RetrievedAt:      retrievedAt,
		})
	}

	return observations, nil
}
