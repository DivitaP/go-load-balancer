package health

import (
	"context"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/DivitaP/go-load-balancer/config"
	"github.com/DivitaP/go-load-balancer/internal/backend"
)

type Checker struct {
	backends  []*backend.Backend
	path      string
	interval  time.Duration
	threshold int
	client    *http.Client
}

func New(backends []*backend.Backend, cfg config.HealthCheckConfig) *Checker {
	return &Checker{
		backends:  backends,
		path:      cfg.Path,
		interval:  time.Duration(cfg.Interval),
		threshold: cfg.Threshold,
		client:    &http.Client{Timeout: time.Duration(cfg.Timeout)},
	}
}

// Start launches the health check loop. It returns immediately;
// the loop stops when ctx is cancelled.
func (c *Checker) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(c.interval)
		defer ticker.Stop()

		c.checkAll(ctx) // immediate first pass, don't wait one interval

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.checkAll(ctx)
			}
		}
	}()
}

// checkAll probes every backend concurrently and waits for all probes.
func (c *Checker) checkAll(ctx context.Context) {
	var wg sync.WaitGroup
	for _, b := range c.backends {
		wg.Add(1)
		go func(b *backend.Backend) {
			defer wg.Done()
			c.check(ctx, b)
		}(b)
	}
	wg.Wait()
}

func (c *Checker) check(ctx context.Context, b *backend.Backend) {
	probeURL := *b.URL // copy so we don't mutate the shared URL
	probeURL.Path = c.path

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, probeURL.String(), nil)
	if err != nil {
		c.fail(b)
		return
	}

	resp, err := c.client.Do(req)
	if err != nil {
		c.fail(b)
		return
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body) // drain so the connection is reused

	if resp.StatusCode >= 400 {
		c.fail(b)
		return
	}

	wasDown := !b.IsAlive()
	b.RecordSuccess()
	if wasDown {
		log.Printf("health: backend %s marked UP", b.URL)
	}
}

func (c *Checker) fail(b *backend.Backend) {
	if b.RecordFailure(c.threshold) {
		log.Printf("health: backend %s marked DOWN", b.URL)
	}
}
