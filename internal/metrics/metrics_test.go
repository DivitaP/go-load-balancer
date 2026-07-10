package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DivitaP/go-load-balancer/internal/backend"
)

func TestMetricsOutput(t *testing.T) {
	b, _ := backend.New("http://localhost:8081", 1)
	b.TotalRequests.Add(42)
	b.TotalErrors.Add(3)
	b.SetAlive(false)

	rec := httptest.NewRecorder()
	Handler([]*backend.Backend{b})(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	body := rec.Body.String()
	for _, want := range []string{
		`lb_backend_up{backend="localhost:8081"} 0`,
		`lb_requests_total{backend="localhost:8081"} 42`,
		`lb_errors_total{backend="localhost:8081"} 3`,
		"# TYPE lb_requests_total counter",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics output missing %q\ngot:\n%s", want, body)
		}
	}
}
