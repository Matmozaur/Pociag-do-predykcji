// Package config reads the api service configuration from the environment.
package config

import (
	"fmt"
	"os"
)

type Config struct {
	HTTPAddr     string // e.g. ":8080"
	DatabaseDSN  string
	PredictorURL string // base URL of the predictor service, e.g. "http://predictor:8000"
	OTLPEndpoint string // optional; tracing is disabled when empty
}

func Load() (Config, error) {
	var cfg Config
	required := []struct {
		name   string
		target *string
	}{{"HTTP_ADDR", &cfg.HTTPAddr}, {"DATABASE_DSN", &cfg.DatabaseDSN}, {"PREDICTOR_URL", &cfg.PredictorURL}}
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
