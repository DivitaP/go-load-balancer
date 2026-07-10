package proxy

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DivitaP/go-load-balancer/internal/backend"
	"github.com/DivitaP/go-load-balancer/internal/balancer"
)

func newTestBackend(t *testing.T, id string, gotXFF *string) *backend.Backend {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gotXFF != nil {
			*gotXFF = r.Header.Get("X-Forwarded-For")
		}
		fmt.Fprint(w, id)
	}))
	t.Cleanup(srv.Close)
	b, err := backend.New(srv.URL, 1)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestProxyForwardsToBackend(t *testing.T) {
	b := newTestBackend(t, "b1", nil)
	lb := New([]*backend.Backend{b}, &balancer.RoundRobin{})

	req := httptest.NewRequest(http.MethodGet, "/hello", nil)
	rec := httptest.NewRecorder()
	lb.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if rec.Body.String() != "b1" {
		t.Errorf("body = %q, want b1", rec.Body.String())
	}
}

func TestProxySetsXForwardedFor(t *testing.T) {
	var xff string
	b := newTestBackend(t, "b1", &xff)
	lb := New([]*backend.Backend{b}, &balancer.RoundRobin{})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.9:5000"
	lb.ServeHTTP(httptest.NewRecorder(), req)

	if xff != "203.0.113.9" {
		t.Errorf("X-Forwarded-For = %q, want client IP", xff)
	}
}

func TestProxyRotatesBackends(t *testing.T) {
	b1 := newTestBackend(t, "b1", nil)
	b2 := newTestBackend(t, "b2", nil)
	lb := New([]*backend.Backend{b1, b2}, &balancer.RoundRobin{})

	seen := map[string]int{}
	for i := 0; i < 4; i++ {
		rec := httptest.NewRecorder()
		lb.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		seen[rec.Body.String()]++
	}
	if seen["b1"] != 2 || seen["b2"] != 2 {
		t.Errorf("distribution = %v, want 2/2", seen)
	}
}

func Test503WhenNoBackendsAlive(t *testing.T) {
	b := newTestBackend(t, "b1", nil)
	b.SetAlive(false)
	lb := New([]*backend.Backend{b}, &balancer.RoundRobin{})

	rec := httptest.NewRecorder()
	lb.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
}

func Test502WhenBackendUnreachable(t *testing.T) {
	// Port 1 refuses connections immediately on localhost.
	b, _ := backend.New("http://127.0.0.1:1", 1)
	lb := New([]*backend.Backend{b}, &balancer.RoundRobin{})

	rec := httptest.NewRecorder()
	lb.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rec.Code)
	}
	if b.TotalErrors.Load() != 1 {
		t.Errorf("errors = %d, want 1", b.TotalErrors.Load())
	}
	if b.ActiveConns() != 0 {
		t.Errorf("conns leaked on error path: %d", b.ActiveConns())
	}
}

func TestLatencyRecordedAfterRequest(t *testing.T) {
	b := newTestBackend(t, "b1", nil)
	lb := New([]*backend.Backend{b}, &balancer.RoundRobin{})
	lb.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if b.AvgLatency() == 0 {
		t.Error("latency was not recorded")
	}
}
