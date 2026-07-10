package dashboard

import (
	"context"
	"sync"
	"time"

	"github.com/DivitaP/go-load-balancer/internal/backend"
)

type BackendSnapshot struct {
	Backend     string  `json:"backend"`
	Alive       bool    `json:"alive"`
	Weight      int     `json:"weight"`
	ActiveConns int64   `json:"active_conns"`
	Requests    uint64  `json:"requests"`
	Errors      uint64  `json:"errors"`
	LatencyMs   float64 `json:"latency_ms"`
}

type Sample struct {
	TS       int64             `json:"ts"` // unix millis
	Backends []BackendSnapshot `json:"backends"`
}

// History is a fixed-size ring buffer of samples.
// Bounded memory: old samples are overwritten, never grown.
type History struct {
	mu       sync.RWMutex
	buf      []Sample
	next     int
	filled   bool
	backends []*backend.Backend
}

func NewHistory(backends []*backend.Backend, capacity int) *History {
	return &History{buf: make([]Sample, capacity), backends: backends}
}

func Snapshot(backends []*backend.Backend) Sample {
	out := make([]BackendSnapshot, 0, len(backends))
	for _, b := range backends {
		out = append(out, BackendSnapshot{
			Backend:     b.URL.Host,
			Alive:       b.IsAlive(),
			Weight:      b.Weight,
			ActiveConns: b.ActiveConns(),
			Requests:    b.TotalRequests.Load(),
			Errors:      b.TotalErrors.Load(),
			LatencyMs:   float64(b.AvgLatency().Microseconds()) / 1000.0,
		})
	}
	return Sample{TS: time.Now().UnixMilli(), Backends: out}
}

func (h *History) record() {
	s := Snapshot(h.backends)
	h.mu.Lock()
	h.buf[h.next] = s
	h.next = (h.next + 1) % len(h.buf)
	if h.next == 0 {
		h.filled = true
	}
	h.mu.Unlock()
}

// Samples returns history in chronological order.
func (h *History) Samples() []Sample {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if !h.filled {
		out := make([]Sample, h.next)
		copy(out, h.buf[:h.next])
		return out
	}
	out := make([]Sample, 0, len(h.buf))
	out = append(out, h.buf[h.next:]...)
	out = append(out, h.buf[:h.next]...)
	return out
}

// Start samples on an interval until ctx is cancelled.
func (h *History) Start(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		h.record()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				h.record()
			}
		}
	}()
}
