// Package config reads the api service configuration from the environment.
package config

import (
	"fmt"
	"os"
)

type Config struct {
	HTTPAddr     string // e.g. ":8080"
	DatabaseDSN  string
	OTLPEndpoint string // optional; tracing is disabled when empty
}

func Load() (Config, error) {
	var cfg Config
	required := []struct {
		name   string
		target *string
	}{{"HTTP_ADDR", &cfg.HTTPAddr}, {"DATABASE_DSN", &cfg.DatabaseDSN}}
	for _, env := range required {
		value, ok := os.LookupEnv(env.name)
		if !ok || value == "" {
			return Config{}, fmt.Errorf("load config: %s is required", env.name)
		}
		*env.target = value
	}
	cfg.OTLPEndpoint = os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	return cfg, nil
}
