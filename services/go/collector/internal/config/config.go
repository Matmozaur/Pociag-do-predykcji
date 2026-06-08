package config

import (
	"fmt"
	"os"
)

type Config struct {
	HTTPAddr             string
	DatabaseDSN          string
	OTLPExporterEndpoint string
	PLKBaseURL           string
	PLKAPIKey            string
}

func Load() (Config, error) {
	httpAddr, ok := os.LookupEnv("HTTP_ADDR")
	if !ok || httpAddr == "" {
		return Config{}, fmt.Errorf("load config: HTTP_ADDR is required")
	}

	databaseDSN, ok := os.LookupEnv("DATABASE_DSN")
	if !ok || databaseDSN == "" {
		return Config{}, fmt.Errorf("load config: DATABASE_DSN is required")
	}

	otlpEndpoint, ok := os.LookupEnv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if !ok || otlpEndpoint == "" {
		return Config{}, fmt.Errorf("load config: OTEL_EXPORTER_OTLP_ENDPOINT is required")
	}

	plkBaseURL, ok := os.LookupEnv("PLK_BASE_URL")
	if !ok || plkBaseURL == "" {
		return Config{}, fmt.Errorf("load config: PLK_BASE_URL is required")
	}

	plkAPIKey, ok := os.LookupEnv("PLK_API_KEY")
	if !ok || plkAPIKey == "" {
		return Config{}, fmt.Errorf("load config: PLK_API_KEY is required")
	}

	return Config{
		HTTPAddr:             httpAddr,
		DatabaseDSN:          databaseDSN,
		OTLPExporterEndpoint: otlpEndpoint,
		PLKBaseURL:           plkBaseURL,
		PLKAPIKey:            plkAPIKey,
	}, nil
}
