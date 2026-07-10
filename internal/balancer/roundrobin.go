package balancer

import (
	"sync/atomic"

	"github.com/DivitaP/go-load-balancer/internal/backend"
)

type RoundRobin struct {
	counter atomic.Uint64
}

func (rr *RoundRobin) Next(backends []*backend.Backend) *backend.Backend {
	n := len(backends)
	if n == 0 {
		return nil
	}
	start := rr.counter.Add(1)
	// Scan at most n slots so a dead backend is skipped
	// without losing rotation fairness.
	for i := 0; i < n; i++ {
		idx := (start + uint64(i)) % uint64(n)
		if backends[idx].IsAlive() {
			return backends[idx]
		}
	}
	return nil
}
