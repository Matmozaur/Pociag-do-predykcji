package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddr             string
	DataServiceBaseURL   string
	OTLPExporterEndpoint string
	RequestTimeout       time.Duration
	CORSAllowOrigin      string
	RateLimitRPS         float64
	RateLimitBurst       int
	CacheControl         string
}

func Load() (Config, error) {
	httpAddr, ok := os.LookupEnv("HTTP_ADDR")
	if !ok || httpAddr == "" {
		return Config{}, fmt.Errorf("load config: HTTP_ADDR is required")
	}

	dataServiceBaseURL, ok := os.LookupEnv("DATA_SERVICE_BASE_URL")
	if !ok || dataServiceBaseURL == "" {
		return Config{}, fmt.Errorf("load config: DATA_SERVICE_BASE_URL is required")
	}

	otlpEndpoint, ok := os.LookupEnv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if !ok || otlpEndpoint == "" {
		return Config{}, fmt.Errorf("load config: OTEL_EXPORTER_OTLP_ENDPOINT is required")
	}

	requestTimeout, err := readDurationEnv("REQUEST_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, fmt.Errorf("load config: %w", err)
	}

	corsAllowOrigin := readStringEnv("CORS_ALLOW_ORIGIN", "*")
	cacheControl := readStringEnv("CACHE_CONTROL", "no-store")

	rateLimitRPS, err := readFloatEnv("RATE_LIMIT_RPS", 50)
	if err != nil {
		return Config{}, fmt.Errorf("load config: %w", err)
	}
	rateLimitBurst, err := readIntEnv("RATE_LIMIT_BURST", 100)
	if err != nil {
		return Config{}, fmt.Errorf("load config: %w", err)
	}

	return Config{
		HTTPAddr:             httpAddr,
		DataServiceBaseURL:   dataServiceBaseURL,
		OTLPExporterEndpoint: otlpEndpoint,
		RequestTimeout:       requestTimeout,
		CORSAllowOrigin:      corsAllowOrigin,
		RateLimitRPS:         rateLimitRPS,
		RateLimitBurst:       rateLimitBurst,
		CacheControl:         cacheControl,
	}, nil
}

func readStringEnv(name, fallback string) string {
	value, ok := os.LookupEnv(name)
	if !ok || value == "" {
		return fallback
	}
	return value
}

func readDurationEnv(name string, fallback time.Duration) (time.Duration, error) {
	value, ok := os.LookupEnv(name)
	if !ok || value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid duration", name)
	}
	return parsed, nil
}

func readFloatEnv(name string, fallback float64) (float64, error) {
	value, ok := os.LookupEnv(name)
	if !ok || value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid number", name)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("%s must be > 0", name)
	}
	return parsed, nil
}

func readIntEnv(name string, fallback int) (int, error) {
	value, ok := os.LookupEnv(name)
	if !ok || value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid integer", name)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("%s must be > 0", name)
	}
	return parsed, nil
}
