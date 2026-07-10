package proxy

import (
	"log"
	"net/http"
	"net/http/httputil"
	"time"

	"github.com/DivitaP/go-load-balancer/internal/backend"
	"github.com/DivitaP/go-load-balancer/internal/balancer"
)

type LoadBalancer struct {
	backends []*backend.Backend
	strategy balancer.Strategy
}

func New(backends []*backend.Backend, strategy balancer.Strategy) *LoadBalancer {
	for _, b := range backends {
		b.Proxy = buildProxy(b)
	}
	return &LoadBalancer{backends: backends, strategy: strategy}
}

func buildProxy(b *backend.Backend) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		// Rewrite (Go 1.20+) replaces the older Director pattern.
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(b.URL)   // route to this backend
			pr.SetXForwarded() // X-Forwarded-For/Host/Proto
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			b.TotalErrors.Add(1)
			log.Printf("proxy error for backend %s: %v", b.URL, err)
			http.Error(w, "bad gateway", http.StatusBadGateway)
		},
	}
}

// ServeHTTP makes LoadBalancer an http.Handler.
func (lb *LoadBalancer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b := lb.strategy.Next(lb.backends)
	if b == nil {
		http.Error(w, "no backends available", http.StatusServiceUnavailable)
		return
	}

	b.IncConns()
	defer b.DecConns()
	b.TotalRequests.Add(1)

	start := time.Now()
	b.Proxy.ServeHTTP(w, r) // synchronous: returns when response is done
	b.RecordLatency(time.Since(start))
}

func (lb *LoadBalancer) Backends() []*backend.Backend {
	return lb.backends
}
