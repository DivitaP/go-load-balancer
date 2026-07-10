package balancer

import "github.com/DivitaP/go-load-balancer/internal/backend"

type LeastConnections struct{}

func (lc *LeastConnections) Next(backends []*backend.Backend) *backend.Backend {
	var best *backend.Backend
	for _, b := range backends {
		if !b.IsAlive() {
			continue
		}
		if best == nil || b.ActiveConns() < best.ActiveConns() {
			best = b
		}
	}
	return best
}
