package backend

import (
	"net/http/httputil"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

// emaAlpha controls how fast the moving average reacts.
// 0.2 means each new sample contributes 20% to the average.
const emaAlpha = 0.2

type Backend struct {
	URL    *url.URL
	Weight int
	Proxy  *httputil.ReverseProxy

	// mu protects alive, avgLatency, failCount.
	mu         sync.RWMutex
	alive      bool
	avgLatency time.Duration
	failCount  int

	// Lock-free counters: hot path, updated on every request.
	activeConns   atomic.Int64
	TotalRequests atomic.Uint64
	TotalErrors   atomic.Uint64
}

func New(rawURL string, weight int) (*Backend, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if weight < 1 {
		weight = 1
	}
	return &Backend{URL: u, Weight: weight, alive: true}, nil
}

func (b *Backend) IsAlive() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.alive
}

func (b *Backend) SetAlive(alive bool) {
	b.mu.Lock()
	b.alive = alive
	b.mu.Unlock()
}

// RecordFailure increments consecutive failures and marks the
// backend dead once threshold is reached. Returns true if the
// backend transitioned to dead on this call.
func (b *Backend) RecordFailure(threshold int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failCount++
	if b.failCount >= threshold && b.alive {
		b.alive = false
		return true
	}
	return false
}

// RecordSuccess resets the failure count and revives the backend.
func (b *Backend) RecordSuccess() {
	b.mu.Lock()
	b.failCount = 0
	b.alive = true
	b.mu.Unlock()
}

// RecordLatency folds a new sample into the exponential moving average.
func (b *Backend) RecordLatency(sample time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.avgLatency == 0 {
		b.avgLatency = sample
		return
	}
	b.avgLatency = time.Duration(
		emaAlpha*float64(sample) + (1-emaAlpha)*float64(b.avgLatency),
	)
}

func (b *Backend) AvgLatency() time.Duration {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.avgLatency
}

func (b *Backend) IncConns()          { b.activeConns.Add(1) }
func (b *Backend) DecConns()          { b.activeConns.Add(-1) }
func (b *Backend) ActiveConns() int64 { return b.activeConns.Load() }
