package balancer

import (
	"sync/atomic"

	"github.com/DivitaP/go-load-balancer/internal/backend"
)

// Weighted implements weighted round robin by expanding each
// alive backend into weight slots, then rotating over the slots.
type Weighted struct {
	counter atomic.Uint64
}

func (w *Weighted) Next(backends []*backend.Backend) *backend.Backend {
	var expanded []*backend.Backend
	for _, b := range backends {
		if !b.IsAlive() {
			continue
		}
		for i := 0; i < b.Weight; i++ {
			expanded = append(expanded, b)
		}
	}
	if len(expanded) == 0 {
		return nil
	}
	idx := w.counter.Add(1) % uint64(len(expanded))
	return expanded[idx]
}