package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DivitaP/go-load-balancer/internal/backend"
)

func TestSnapshotReflectsBackendState(t *testing.T) {
	b, _ := backend.New("http://localhost:8081", 3)
	b.TotalRequests.Add(10)
	b.TotalErrors.Add(2)
	b.IncConns()
	b.RecordLatency(50 * time.Millisecond)

	s := Snapshot([]*backend.Backend{b})
	got := s.Backends[0]

	if got.Backend != "localhost:8081" || got.Weight != 3 ||
		got.Requests != 10 || got.Errors != 2 || got.ActiveConns != 1 {
		t.Fatalf("snapshot mismatch: %+v", got)
	}
	if got.LatencyMs < 49 || got.LatencyMs > 51 {
		t.Errorf("latency_ms = %v, want ~50", got.LatencyMs)
	}
}

func TestHistoryRingBufferWrapsAndOrders(t *testing.T) {
	b, _ := backend.New("http://localhost:8081", 1)
	h := NewHistory([]*backend.Backend{b}, 3)

	for i := 0; i < 5; i++ {
		b.TotalRequests.Add(1)
		h.record()
	}

	samples := h.Samples()
	if len(samples) != 3 {
		t.Fatalf("len = %d, want capacity 3", len(samples))
	}
	// Oldest two evicted; remaining must be requests 3,4,5 in order.
	want := []uint64{3, 4, 5}
	for i, s := range samples {
		if s.Backends[0].Requests != want[i] {
			t.Errorf("sample[%d] requests = %d, want %d", i, s.Backends[0].Requests, want[i])
		}
	}
}

func TestHistoryPartiallyFilled(t *testing.T) {
	b, _ := backend.New("http://localhost:8081", 1)
	h := NewHistory([]*backend.Backend{b}, 10)
	h.record()
	h.record()
	if len(h.Samples()) != 2 {
		t.Errorf("want 2 samples before wrap, got %d", len(h.Samples()))
	}
}

func TestStatsHandlerJSON(t *testing.T) {
	b, _ := backend.New("http://localhost:8081", 1)
	b.SetAlive(false)
	h := NewHistory([]*backend.Backend{b}, 5)
	h.record()

	rec := httptest.NewRecorder()
	StatsHandler([]*backend.Backend{b}, h)(rec, httptest.NewRequest(http.MethodGet, "/api/stats", nil))

	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type = %q", ct)
	}
	var resp statsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if resp.Current.Backends[0].Alive {
		t.Error("expected alive=false in current snapshot")
	}
	if len(resp.History) != 1 {
		t.Errorf("history len = %d, want 1", len(resp.History))
	}
}

func TestPageHandlerServesEmbeddedHTML(t *testing.T) {
	rec := httptest.NewRecorder()
	PageHandler()(rec, httptest.NewRequest(http.MethodGet, "/dashboard", nil))
	if rec.Code != http.StatusOK || rec.Body.Len() == 0 {
		t.Fatalf("status=%d len=%d", rec.Code, rec.Body.Len())
	}
}
