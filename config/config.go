package config

import (
	"fmt"
	"net/url"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Duration time.Duration

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	*d = Duration(parsed)
	return nil
}

type TLSConfig struct {
	Enabled bool   `yaml:"enabled"`
	Cert    string `yaml:"cert"`
	Key     string `yaml:"key"`
}

type HealthCheckConfig struct {
	Path      string   `yaml:"path"`
	Interval  Duration `yaml:"interval"`
	Threshold int      `yaml:"threshold"`
	Timeout   Duration `yaml:"timeout"`
}

type BackendConfig struct {
	URL    string `yaml:"url"`
	Weight int    `yaml:"weight"`
}

type Config struct {
	Port        int               `yaml:"port"`
	TLS         TLSConfig         `yaml:"tls"`
	Strategy    string            `yaml:"strategy"`
	HealthCheck HealthCheckConfig `yaml:"health_check"`
	Backends    []BackendConfig   `yaml:"backends"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Strategy == "" {
		c.Strategy = "round_robin"
	}
	if c.HealthCheck.Path == "" {
		c.HealthCheck.Path = "/health"
	}
	if c.HealthCheck.Interval == 0 {
		c.HealthCheck.Interval = Duration(10 * time.Second)
	}
	if c.HealthCheck.Timeout == 0 {
		c.HealthCheck.Timeout = Duration(2 * time.Second)
	}
	if c.HealthCheck.Threshold == 0 {
		c.HealthCheck.Threshold = 3
	}
	for i := range c.Backends {
		if c.Backends[i].Weight < 1 {
			c.Backends[i].Weight = 1
		}
	}
}

func (c *Config) validate() error {
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("invalid port: %d", c.Port)
	}
	if len(c.Backends) == 0 {
		return fmt.Errorf("at least one backend is required")
	}

	for _, b := range c.Backends {
		u, err := url.Parse(b.URL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("invalid backend URL: %q", b.URL)
		}
	}

	switch c.Strategy {
	case "round_robin", "least_connections", "weighted":
	default:
		return fmt.Errorf("unknown strategy: %q", c.Strategy)
	}
	if c.TLS.Enabled && (c.TLS.Cert == "" || c.TLS.Key == "") {
		return fmt.Errorf("TLS is enabled but cert or key is missing")
	}
	return nil
}
