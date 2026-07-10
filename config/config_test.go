package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadValid(t *testing.T) {
	path := writeTemp(t, `
port: 8080
strategy: least_connections
health_check:
  path: /health
  interval: 5s
  threshold: 2
backends:
  - url: http://localhost:8081
    weight: 3
  - url: http://localhost:8082
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Strategy != "least_connections" {
		t.Errorf("strategy = %q", cfg.Strategy)
	}
	if time.Duration(cfg.HealthCheck.Interval) != 5*time.Second {
		t.Errorf("interval = %v", cfg.HealthCheck.Interval)
	}
	if cfg.Backends[1].Weight != 1 {
		t.Errorf("default weight not applied, got %d", cfg.Backends[1].Weight)
	}
	if time.Duration(cfg.HealthCheck.Timeout) != 2*time.Second {
		t.Errorf("default timeout not applied")
	}
}

func TestLoadRejectsBadStrategy(t *testing.T) {
	path := writeTemp(t, `
port: 8080
strategy: random_pick
backends:
  - url: http://localhost:8081
`)
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for unknown strategy")
	}
}

func TestLoadRejectsNoBackends(t *testing.T) {
	path := writeTemp(t, `port: 8080`)
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for empty backends")
	}
}

func TestLoadRejectsBadDuration(t *testing.T) {
	path := writeTemp(t, `
port: 8080
health_check:
  interval: banana
backends:
  - url: http://localhost:8081
`)
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for invalid duration")
	}
}
