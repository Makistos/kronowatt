package fmi

import (
	"os"
	"testing"
)

func TestParseStations(t *testing.T) {
	body, err := os.ReadFile("testdata/stations_sample.xml")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	stations, err := parseStations(body)
	if err != nil {
		t.Fatalf("parseStations: %v", err)
	}
	if len(stations) != 5 {
		t.Fatalf("got %d stations, want 5", len(stations))
	}

	var oulu *Station
	for i := range stations {
		if stations[i].FMISID == "101786" {
			oulu = &stations[i]
		}
	}
	if oulu == nil {
		t.Fatal("station 101786 (Oulu lentoasema) not found")
	}
	if oulu.Name != "Oulu lentoasema" {
		t.Errorf("Name = %q, want %q", oulu.Name, "Oulu lentoasema")
	}
	if oulu.Latitude != 64.935034 || oulu.Longitude != 25.339195 {
		t.Errorf("coords = (%v, %v), want (64.935034, 25.339195)", oulu.Latitude, oulu.Longitude)
	}
}

func TestParseNearbyCoords(t *testing.T) {
	body, err := os.ReadFile("testdata/nearby_observations_sample.xml")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	coords, err := parseNearbyCoords(body)
	if err != nil {
		t.Fatalf("parseNearbyCoords: %v", err)
	}
	if len(coords) != 7 {
		t.Fatalf("got %d distinct coords, want 7", len(coords))
	}
	if !coords[coordKey(64.93503, 25.33920)] {
		t.Error("expected Oulu lentoasema's coordinate (64.93503, 25.33920) to be present")
	}
}

func TestFindNearbyStationsRanking(t *testing.T) {
	stationsBody, err := os.ReadFile("testdata/stations_sample.xml")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	stations, err := parseStations(stationsBody)
	if err != nil {
		t.Fatalf("parseStations: %v", err)
	}

	nearbyBody, err := os.ReadFile("testdata/nearby_observations_sample.xml")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	coords, err := parseNearbyCoords(nearbyBody)
	if err != nil {
		t.Fatalf("parseNearbyCoords: %v", err)
	}

	// Exercises the same ranking logic as FindNearbyStations without the
	// network round trip: querying from Oulu lentoasema's own coordinates
	// should rank Oulu lentoasema itself first, at ~0 distance.
	lat, lon := 64.935034, 25.339195
	var results []StationDistance
	for _, s := range stations {
		if !coords[coordKey(s.Latitude, s.Longitude)] {
			continue
		}
		results = append(results, StationDistance{Station: s, DistanceKM: haversineKM(lat, lon, s.Latitude, s.Longitude)})
	}
	if len(results) == 0 {
		t.Fatal("no stations matched between the two fixtures")
	}

	nearest := results[0]
	for _, r := range results {
		if r.DistanceKM < nearest.DistanceKM {
			nearest = r
		}
	}
	if nearest.FMISID != "101786" {
		t.Errorf("nearest station = %q (%s), want 101786 (Oulu lentoasema)", nearest.FMISID, nearest.Name)
	}
	if nearest.DistanceKM > 0.01 {
		t.Errorf("distance to self = %v km, want ~0", nearest.DistanceKM)
	}
}
