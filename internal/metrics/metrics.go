package metrics

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/DivitaP/go-load-balancer/internal/backend"
)

// Handler renders backend state in the Prometheus text exposition
// format, written by hand instead of using client_golang.
func Handler(backends []*backend.Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var sb strings.Builder

		sb.WriteString("# HELP lb_backend_up Backend health (1 = up, 0 = down)\n")
		sb.WriteString("# TYPE lb_backend_up gauge\n")
		for _, b := range backends {
			up := 0
			if b.IsAlive() {
				up = 1
			}
			fmt.Fprintf(&sb, "lb_backend_up{backend=%q} %d\n", b.URL.Host, up)
		}

		sb.WriteString("# HELP lb_requests_total Total requests proxied per backend\n")
		sb.WriteString("# TYPE lb_requests_total counter\n")
		for _, b := range backends {
			fmt.Fprintf(&sb, "lb_requests_total{backend=%q} %d\n", b.URL.Host, b.TotalRequests.Load())
		}

		sb.WriteString("# HELP lb_errors_total Total proxy errors per backend\n")
		sb.WriteString("# TYPE lb_errors_total counter\n")
		for _, b := range backends {
			fmt.Fprintf(&sb, "lb_errors_total{backend=%q} %d\n", b.URL.Host, b.TotalErrors.Load())
		}

		sb.WriteString("# HELP lb_active_connections In-flight requests per backend\n")
		sb.WriteString("# TYPE lb_active_connections gauge\n")
		for _, b := range backends {
			fmt.Fprintf(&sb, "lb_active_connections{backend=%q} %d\n", b.URL.Host, b.ActiveConns())
		}

		sb.WriteString("# HELP lb_backend_latency_seconds EMA of request latency per backend\n")
		sb.WriteString("# TYPE lb_backend_latency_seconds gauge\n")
		for _, b := range backends {
			fmt.Fprintf(&sb, "lb_backend_latency_seconds{backend=%q} %f\n", b.URL.Host, b.AvgLatency().Seconds())
		}

		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		w.Write([]byte(sb.String()))
	}
}
