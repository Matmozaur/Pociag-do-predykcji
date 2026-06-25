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
	S3Endpoint           string
	S3Bucket             string
	S3AccessKey          string
	S3SecretKey          string
	S3UsePathStyle       bool
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

	s3Endpoint, ok := os.LookupEnv("S3_ENDPOINT")
	if !ok || s3Endpoint == "" {
		return Config{}, fmt.Errorf("load config: S3_ENDPOINT is required")
	}

	s3Bucket, ok := os.LookupEnv("S3_BUCKET")
	if !ok || s3Bucket == "" {
		return Config{}, fmt.Errorf("load config: S3_BUCKET is required")
	}

	s3AccessKey, ok := os.LookupEnv("S3_ACCESS_KEY")
	if !ok || s3AccessKey == "" {
		return Config{}, fmt.Errorf("load config: S3_ACCESS_KEY is required")
	}

	s3SecretKey, ok := os.LookupEnv("S3_SECRET_KEY")
	if !ok || s3SecretKey == "" {
		return Config{}, fmt.Errorf("load config: S3_SECRET_KEY is required")
	}

	s3UsePathStyle := os.Getenv("S3_USE_PATH_STYLE") == "true"

	return Config{
		HTTPAddr:             httpAddr,
		DatabaseDSN:          databaseDSN,
		OTLPExporterEndpoint: otlpEndpoint,
		PLKBaseURL:           plkBaseURL,
		PLKAPIKey:            plkAPIKey,
		S3Endpoint:           s3Endpoint,
		S3Bucket:             s3Bucket,
		S3AccessKey:          s3AccessKey,
		S3SecretKey:          s3SecretKey,
		S3UsePathStyle:       s3UsePathStyle,
	}, nil
}
