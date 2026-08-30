package weather

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestParseObservations(t *testing.T) {
	body, err := os.ReadFile("testdata/observations_101786.xml")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	obs, err := parseObservations(body, "101786")
	if err != nil {
		t.Fatalf("parseObservations: %v", err)
	}

	if len(obs) != 4 {
		t.Fatalf("got %d observations, want 4", len(obs))
	}

	first := obs[0]
	wantTime, _ := time.Parse(time.RFC3339, "2026-08-30T17:00:00Z")
	if !first.Time.Equal(wantTime) {
		t.Errorf("first observation time = %v, want %v", first.Time, wantTime)
	}
	if first.StationFMISID != "101786" {
		t.Errorf("StationFMISID = %q, want 101786", first.StationFMISID)
	}
	if first.AirTemperature == nil || *first.AirTemperature != 16.8 {
		t.Errorf("AirTemperature = %v, want 16.8", first.AirTemperature)
	}
	if first.RelativeHumidity == nil || *first.RelativeHumidity != 85.0 {
		t.Errorf("RelativeHumidity = %v, want 85.0", first.RelativeHumidity)
	}
	if first.AirPressure == nil || *first.AirPressure != 1004.6 {
		t.Errorf("AirPressure = %v, want 1004.6", first.AirPressure)
	}

	// r_1h is "NaN" in the fixture at this timestamp — must come through as
	// a nil field (no reading), not a NaN float, and must not appear in
	// RawPayload (which would otherwise fail to marshal).
	if first.Precipitation != nil {
		t.Errorf("Precipitation = %v, want nil (source reported NaN)", *first.Precipitation)
	}
	if string(first.RawPayload) == "" {
		t.Fatal("RawPayload is empty")
	}
	var raw map[string]float64
	if err := json.Unmarshal(first.RawPayload, &raw); err != nil {
		t.Fatalf("unmarshal RawPayload: %v", err)
	}
	if _, ok := raw["r_1h"]; ok {
		t.Error("RawPayload contains r_1h, want it dropped (NaN in source)")
	}
	if v, ok := raw["t2m"]; !ok || v != 16.8 {
		t.Errorf("RawPayload t2m = %v, want 16.8", v)
	}

	// times must be strictly ascending
	for i := 1; i < len(obs); i++ {
		if !obs[i].Time.After(obs[i-1].Time) {
			t.Errorf("observations not sorted ascending at index %d: %v <= %v", i, obs[i].Time, obs[i-1].Time)
		}
	}
}
