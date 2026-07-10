package balancer

import (
	"fmt"
	"github.com/DivitaP/go-load-balancer/internal/backend"
)

// Strategy picks the next backend for a request.
// Implementations must be safe for concurrent use.
type Strategy interface {
	Next(backends []*backend.Backend) *backend.Backend
}

func New(name string) (Strategy, error) {
	switch name {
	case "round_robin":
		return &RoundRobin{}, nil
	case "least_connections":
		return &LeastConnections{}, nil
	case "weighted":
		return &Weighted{}, nil
	default:
		return nil, fmt.Errorf("unknown strategy: %q", name)
	}
}
