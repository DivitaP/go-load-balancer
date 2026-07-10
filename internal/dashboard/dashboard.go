package dashboard

import (
	"embed"
	"encoding/json"
	"net/http"

	"github.com/DivitaP/go-load-balancer/internal/backend"
)

var assets embed.FS

type statsResponse struct {
	Current Sample   `json:"current"`
	History []Sample `json:"history"`
}

// StatsHandler serves the JSON the dashboard polls.
func StatsHandler(backends []*backend.Backend, h *History) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := statsResponse{
			Current: Snapshot(backends),
			History: h.Samples(),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// PageHandler serves the embedded HTML dashboard.
func PageHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := assets.ReadFile("index.html")
		if err != nil {
			http.Error(w, "dashboard unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	}
}
