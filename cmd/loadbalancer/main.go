package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/DivitaP/go-load-balancer/config"
	"github.com/DivitaP/go-load-balancer/internal/backend"
	"github.com/DivitaP/go-load-balancer/internal/balancer"
	"github.com/DivitaP/go-load-balancer/internal/dashboard"
	"github.com/DivitaP/go-load-balancer/internal/health"
	"github.com/DivitaP/go-load-balancer/internal/metrics"
	"github.com/DivitaP/go-load-balancer/internal/proxy"
)

func main() {
	cfgPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	backends := make([]*backend.Backend, 0, len(cfg.Backends))
	for _, bc := range cfg.Backends {
		b, err := backend.New(bc.URL, bc.Weight)
		if err != nil {
			log.Fatalf("backend %q: %v", bc.URL, err)
		}
		backends = append(backends, b)
	}

	strategy, err := balancer.New(cfg.Strategy)
	if err != nil {
		log.Fatal(err)
	}

	lb := proxy.New(backends, strategy)

	// Root context cancelled on Ctrl+C or SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	health.New(backends, cfg.HealthCheck).Start(ctx)

	// 300 samples at 2s = 10 minutes of retained history.
	hist := dashboard.NewHistory(backends, 300)
	hist.Start(ctx, 2*time.Second)

	mux := http.NewServeMux()
	mux.Handle("/metrics", metrics.Handler(backends))
	mux.Handle("/dashboard", dashboard.PageHandler())
	mux.Handle("/api/stats", dashboard.StatsHandler(backends, hist))
	mux.Handle("/", lb) // catch-all: everything else is proxied

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: mux,
	}

	// Graceful shutdown: stop accepting, drain in-flight requests.
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}()

	log.Printf("listening on :%d (strategy=%s, tls=%v)", cfg.Port, cfg.Strategy, cfg.TLS.Enabled)
	if cfg.TLS.Enabled {
		err = srv.ListenAndServeTLS(cfg.TLS.Cert, cfg.TLS.Key)
	} else {
		err = srv.ListenAndServe()
	}
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
