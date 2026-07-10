package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DivitaP/go-load-balancer/config"
	"github.com/DivitaP/go-load-balancer/internal/backend"
)

func newChecker(b *backend.Backend, threshold int) *Checker {
	return New([]*backend.Backend{b}, config.HealthCheckConfig{
		Path:      "/health",
		Interval:  config.Duration(time.Second),
		Threshold: threshold,
		Timeout:   config.Duration(time.Second),
	})
}

func TestBackendMarkedDownAfterThresholdAndRecovers(t *testing.T) {
	var healthy atomic.Bool
	healthy.Store(true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if healthy.Load() {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	b, _ := backend.New(srv.URL, 1)
	c := newChecker(b, 2)
	ctx := context.Background()

	c.checkAll(ctx)
	if !b.IsAlive() {
		t.Fatal("healthy backend marked down")
	}

	healthy.Store(false)
	c.checkAll(ctx)
	if !b.IsAlive() {
		t.Fatal("marked down after 1 failure, threshold is 2")
	}
	c.checkAll(ctx)
	if b.IsAlive() {
		t.Fatal("not marked down after reaching threshold")
	}

	healthy.Store(true)
	c.checkAll(ctx)
	if !b.IsAlive() {
		t.Fatal("did not recover after successful probe")
	}
}

func TestUnreachableBackendMarkedDown(t *testing.T) {
	b, _ := backend.New("http://127.0.0.1:1", 1)
	c := newChecker(b, 1)
	c.checkAll(context.Background())
	if b.IsAlive() {
		t.Fatal("unreachable backend still alive")
	}
}

func TestStartStopsOnContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	b, _ := backend.New(srv.URL, 1)

	c := New([]*backend.Backend{b}, config.HealthCheckConfig{
		Path:      "/health",
		Interval:  config.Duration(10 * time.Millisecond),
		Threshold: 1,
		Timeout:   config.Duration(time.Second),
	})

	ctx, cancel := context.WithCancel(context.Background())
	c.Start(ctx)
	time.Sleep(30 * time.Millisecond)
	cancel()
	time.Sleep(30 * time.Millisecond) // loop should have exited; -race verifies no activity
}
