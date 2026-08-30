package api

import (
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// parseDateRange supports either ?year=2025 (convenience — maps to
// [Jan 1, Jan 1 next year) UTC, mirroring how the frontend's fake-data
// fixtures are split one file per year) or explicit ?start=...&end=...
// RFC3339 timestamps, for any endpoint querying a time-series table.
func parseDateRange(r *http.Request) (start, end time.Time, err error) {
	q := r.URL.Query()

	if y := q.Get("year"); y != "" {
		year, convErr := strconv.Atoi(y)
		if convErr != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid year %q", y)
		}
		start = time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
		end = time.Date(year+1, 1, 1, 0, 0, 0, 0, time.UTC)
		return start, end, nil
	}

	startStr, endStr := q.Get("start"), q.Get("end")
	if startStr == "" || endStr == "" {
		return time.Time{}, time.Time{}, fmt.Errorf("provide either ?year= or both ?start= and ?end= (RFC3339)")
	}
	start, err = time.Parse(time.RFC3339, startStr)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid start: %w", err)
	}
	end, err = time.Parse(time.RFC3339, endStr)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid end: %w", err)
	}
	return start, end, nil
}
