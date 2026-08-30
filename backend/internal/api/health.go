package api

import (
	"net/http"
	"syscall"
	"time"

	"kronowatt/backend/internal/storage"
)

type collectorStatusDTO struct {
	Name          string     `json:"name"`
	Status        string     `json:"status"`
	LastSuccessAt *time.Time `json:"last_success_at,omitempty"`
	LastError     *string    `json:"last_error,omitempty"`
	LastErrorAt   *time.Time `json:"last_error_at,omitempty"`
}

type healthResponse struct {
	Status      string               `json:"status"`
	DiskUsedPct float64              `json:"disk_used_pct"`
	Collectors  []collectorStatusDTO `json:"collectors"`
}

// diskDegradedPct mirrors spec §9a/§44.17: the health endpoint must flag
// "degraded" above ~80% disk usage.
const diskDegradedPct = 80.0

func healthHandler(repo *storage.CollectorRepository, diskCheckPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		collectors, err := repo.List(r.Context())
		if err != nil {
			http.Error(w, "failed to load collector status", http.StatusInternalServerError)
			return
		}

		diskPct, diskErr := diskUsedPercent(diskCheckPath)

		status := "ok"
		for _, c := range collectors {
			if c.Status == "error" {
				status = "degraded"
			}
		}
		if diskErr == nil && diskPct >= diskDegradedPct {
			status = "degraded"
		}

		dtos := make([]collectorStatusDTO, len(collectors))
		for i, c := range collectors {
			dtos[i] = collectorStatusDTO{
				Name:          c.Name,
				Status:        c.Status,
				LastSuccessAt: c.LastSuccessAt,
				LastError:     c.LastError,
				LastErrorAt:   c.LastErrorAt,
			}
		}

		writeJSON(w, http.StatusOK, healthResponse{
			Status:      status,
			DiskUsedPct: diskPct,
			Collectors:  dtos,
		})
	}
}

func diskUsedPercent(path string) (float64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	total := float64(stat.Blocks) * float64(stat.Bsize)
	if total == 0 {
		return 0, nil
	}
	free := float64(stat.Bfree) * float64(stat.Bsize)
	return (total - free) / total * 100, nil
}
