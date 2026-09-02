// Station discovery for the home-location settings UI: given a
// latitude/longitude, find the nearest FMI weather observation stations to
// suggest for KRONOWATT_FMI_STATION_FMISID. Separate from the observation
// fetch/parse logic in fmi.go (different stored queries, different
// response shapes) but the same package, since both are FMI-specific.
package fmi

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Station is a named, located FMI weather observation station.
type Station struct {
	FMISID    string
	Name      string
	Latitude  float64
	Longitude float64
}

// StationDistance pairs a Station with its distance from a query point.
type StationDistance struct {
	Station
	DistanceKM float64
}

// --- fmi::ef::stations response shape (station metadata: fmisid, name,
// coordinates — everything the settings UI needs to label a choice) ---

type stationsFeatureCollection struct {
	Members []stationsMember `xml:"member"`
}

type stationsMember struct {
	Facility facility `xml:"EnvironmentalMonitoringFacility"`
}

type facility struct {
	Identifier string    `xml:"identifier"`
	Names      []gmlName `xml:"name"`
	Point      gmlPoint  `xml:"representativePoint>Point"`
}

type gmlName struct {
	CodeSpace string `xml:"codeSpace,attr"`
	Value     string `xml:",chardata"`
}

type gmlPoint struct {
	Pos string `xml:"pos"` // "lat lon", per axisLabels="Lat Long" on the real response
}

// point parses "lat lon" (whitespace-separated, per axisLabels="Lat Long"
// on both response shapes this package parses).
func (p gmlPoint) point() (lat, lon float64, ok bool) {
	parts := strings.Fields(p.Pos)
	if len(parts) != 2 {
		return 0, 0, false
	}
	lat, errLat := strconv.ParseFloat(parts[0], 64)
	lon, errLon := strconv.ParseFloat(parts[1], 64)
	return lat, lon, errLat == nil && errLon == nil
}

func (f facility) stationName() string {
	for _, n := range f.Names {
		if strings.HasSuffix(n.CodeSpace, "/locationcode/name") {
			return strings.TrimSpace(n.Value)
		}
	}
	return ""
}

// fetchAllStations fetches FMI's full station metadata list — every
// network, not just weather stations, since some weather stations (e.g.
// airport-operated ones like Oulu lentoasema/101786) belong to networks
// that aren't obviously "weather" by id alone. Filtering to only stations
// that are *currently reporting* happens separately, in
// nearbyReportingCoords — this list is purely for resolving a reporting
// coordinate back to a human name and FMISID.
func fetchAllStations(ctx context.Context, client *http.Client) ([]Station, error) {
	reqURL := baseURL + "?" + url.Values{
		"service":        {"WFS"},
		"version":        {"2.0.0"},
		"request":        {"getFeature"},
		"storedquery_id": {"fmi::ef::stations"},
	}.Encode()

	body, err := getBody(ctx, client, reqURL)
	if err != nil {
		return nil, err
	}
	return parseStations(body)
}

func parseStations(body []byte) ([]Station, error) {
	var fc stationsFeatureCollection
	if err := xml.Unmarshal(body, &fc); err != nil {
		return nil, fmt.Errorf("parse stations response: %w", err)
	}

	stations := make([]Station, 0, len(fc.Members))
	for _, m := range fc.Members {
		f := m.Facility
		name := f.stationName()
		lat, lon, ok := f.Point.point()
		if f.Identifier == "" || name == "" || !ok {
			continue
		}
		stations = append(stations, Station{FMISID: f.Identifier, Name: name, Latitude: lat, Longitude: lon})
	}
	return stations, nil
}

// --- fmi::observations::weather::simple, bbox-restricted, used only to
// discover which coordinates are *actually* reporting right now (the
// station metadata list includes non-weather station types, e.g. sea-level
// gauges, that fetchAllStations can't distinguish by id/name alone) ---

type nearbyFeatureCollection struct {
	Members []nearbyMember `xml:"member"`
}

type nearbyMember struct {
	Element nearbyElement `xml:"BsWfsElement"`
}

type nearbyElement struct {
	Point gmlPoint `xml:"Location>Point"`
}

// nearbyReportingCoords returns the distinct (lat, lon) coordinates of
// stations that returned a temperature reading inside the given bounding
// box within the last 20 minutes. Restricted to a single parameter
// (t2m — reported by every weather station) and a narrow time window
// purely to keep the response small: the unrestricted "simple" query
// returns every parameter over its full ~12h default window, which is
// tens of MB for a box this size — confirmed live (14MB unrestricted vs
// ~9KB restricted for the same box).
func nearbyReportingCoords(ctx context.Context, client *http.Client, minLat, minLon, maxLat, maxLon float64) (map[string]bool, error) {
	start := time.Now().UTC().Add(-20 * time.Minute).Format(time.RFC3339)
	reqURL := baseURL + "?" + url.Values{
		"service":        {"WFS"},
		"version":        {"2.0.0"},
		"request":        {"getFeature"},
		"storedquery_id": {"fmi::observations::weather::simple"},
		"bbox":           {fmt.Sprintf("%f,%f,%f,%f", minLon, minLat, maxLon, maxLat)},
		"parameters":     {"t2m"},
		"starttime":      {start},
	}.Encode()

	body, err := getBody(ctx, client, reqURL)
	if err != nil {
		return nil, err
	}
	return parseNearbyCoords(body)
}

func parseNearbyCoords(body []byte) (map[string]bool, error) {
	var fc nearbyFeatureCollection
	if err := xml.Unmarshal(body, &fc); err != nil {
		return nil, fmt.Errorf("parse nearby-observations response: %w", err)
	}

	coords := make(map[string]bool)
	for _, m := range fc.Members {
		lat, lon, ok := m.Element.Point.point()
		if !ok {
			continue
		}
		coords[coordKey(lat, lon)] = true
	}
	return coords, nil
}

// coordKey rounds to 5 decimals (~1m precision) so the same station's
// coordinate compares equal across responses despite minor float
// formatting differences.
func coordKey(lat, lon float64) string {
	return fmt.Sprintf("%.5f,%.5f", lat, lon)
}

func haversineKM(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusKM = 6371.0
	toRad := func(d float64) float64 { return d * math.Pi / 180 }
	dLat := toRad(lat2 - lat1)
	dLon := toRad(lon2 - lon1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(toRad(lat1))*math.Cos(toRad(lat2))*math.Sin(dLon/2)*math.Sin(dLon/2)
	return earthRadiusKM * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// FindNearbyStations returns up to n stations near (lat, lon), nearest
// first, restricted to stations that are *currently reporting* live
// weather data — not just any entry in FMI's station metadata, which also
// covers non-weather station types (sea-level gauges, hydrological
// stations, ...) that would silently never produce readings for the
// collector if picked. Starts with a ~50-100km box and doubles it (up to 4
// times) until at least n reporting stations are found — Lapland's sparser
// station network needs a wider search than the coast does.
func FindNearbyStations(ctx context.Context, client *http.Client, lat, lon float64, n int) ([]StationDistance, error) {
	allStations, err := fetchAllStations(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("fetch station list: %w", err)
	}

	dLat, dLon := 0.5, 1.0
	var coords map[string]bool
	for attempt := 0; attempt < 5; attempt++ {
		coords, err = nearbyReportingCoords(ctx, client, lat-dLat, lon-dLon, lat+dLat, lon+dLon)
		if err != nil {
			return nil, fmt.Errorf("fetch nearby observations: %w", err)
		}
		if len(coords) >= n {
			break
		}
		dLat *= 2
		dLon *= 2
	}

	var results []StationDistance
	for _, s := range allStations {
		if !coords[coordKey(s.Latitude, s.Longitude)] {
			continue
		}
		results = append(results, StationDistance{
			Station:    s,
			DistanceKM: haversineKM(lat, lon, s.Latitude, s.Longitude),
		})
	}

	sort.Slice(results, func(i, j int) bool { return results[i].DistanceKM < results[j].DistanceKM })
	if len(results) > n {
		results = results[:n]
	}
	return results, nil
}

func getBody(ctx context.Context, client *http.Client, reqURL string) ([]byte, error) {
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
	return io.ReadAll(resp.Body)
}
