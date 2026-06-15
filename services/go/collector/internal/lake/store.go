package lake

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

type Config struct {
	Endpoint     string
	Bucket       string
	AccessKey    string
	SecretKey    string
	UsePathStyle bool
}

type Store struct {
	client *s3.Client
	bucket string
	tracer trace.Tracer
}

func New(cfg Config) *Store {
	client := s3.New(s3.Options{
		BaseEndpoint: aws.String(cfg.Endpoint),
		Region:       "us-east-1",
		Credentials:  credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		UsePathStyle: cfg.UsePathStyle,
	})

	return &Store{
		client: client,
		bucket: cfg.Bucket,
		tracer: otel.Tracer("pociag.collector"),
	}
}

func (s *Store) Ping(ctx context.Context) error {
	ctx, span := s.tracer.Start(ctx, "lake.ping")
	defer span.End()

	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(s.bucket),
	})
	if err != nil {
		return fmt.Errorf("lake ping: %w", err)
	}
	return nil
}

func (s *Store) PutRawDictionaries(ctx context.Context, dictionaryType string, payload []byte, recordCount int, runID int64) (string, error) {
	ctx, span := s.tracer.Start(ctx, "lake.dictionaries.put")
	defer span.End()

	now := time.Now().UTC()
	key := fmt.Sprintf("raw/dictionaries/%d/%02d/%02d/run_%d_%s.parquet",
		now.Year(), now.Month(), now.Day(), runID, dictionaryType)

	data := wrapAsParquetJSON(payload, map[string]string{
		"dictionary_type":  dictionaryType,
		"record_count":     fmt.Sprintf("%d", recordCount),
		"ingestion_run_id": fmt.Sprintf("%d", runID),
		"fetched_at":       now.Format(time.RFC3339),
	})

	if err := s.putObject(ctx, key, data); err != nil {
		return "", fmt.Errorf("put raw dictionaries: %w", err)
	}

	return key, nil
}

func (s *Store) PutRawSchedules(ctx context.Context, dateFrom time.Time, dateTo time.Time, page int, payload []byte, recordCount int, runID int64) (string, error) {
	ctx, span := s.tracer.Start(ctx, "lake.schedules.put")
	defer span.End()

	key := fmt.Sprintf("raw/schedules/%d/%02d/%02d/run_%d_page_%d.parquet",
		dateFrom.Year(), dateFrom.Month(), dateFrom.Day(), runID, page)

	data := wrapAsParquetJSON(payload, map[string]string{
		"date_from":        dateFrom.Format("2006-01-02"),
		"date_to":          dateTo.Format("2006-01-02"),
		"page":             fmt.Sprintf("%d", page),
		"record_count":     fmt.Sprintf("%d", recordCount),
		"ingestion_run_id": fmt.Sprintf("%d", runID),
		"fetched_at":       time.Now().UTC().Format(time.RFC3339),
	})

	if err := s.putObject(ctx, key, data); err != nil {
		return "", fmt.Errorf("put raw schedules: %w", err)
	}

	return key, nil
}

func (s *Store) PutRawOperations(ctx context.Context, operatingDate time.Time, page int, payload []byte, recordCount int, runID int64) (string, error) {
	ctx, span := s.tracer.Start(ctx, "lake.operations.put")
	defer span.End()

	key := fmt.Sprintf("raw/operations/%d/%02d/%02d/run_%d_page_%d.parquet",
		operatingDate.Year(), operatingDate.Month(), operatingDate.Day(), runID, page)

	data := wrapAsParquetJSON(payload, map[string]string{
		"operating_date":   operatingDate.Format("2006-01-02"),
		"page":             fmt.Sprintf("%d", page),
		"record_count":     fmt.Sprintf("%d", recordCount),
		"ingestion_run_id": fmt.Sprintf("%d", runID),
		"fetched_at":       time.Now().UTC().Format(time.RFC3339),
	})

	if err := s.putObject(ctx, key, data); err != nil {
		return "", fmt.Errorf("put raw operations: %w", err)
	}

	return key, nil
}

func (s *Store) PutRawDisruptions(ctx context.Context, dateFrom time.Time, dateTo time.Time, payload []byte, recordCount int, runID int64) (string, error) {
	ctx, span := s.tracer.Start(ctx, "lake.disruptions.put")
	defer span.End()

	key := fmt.Sprintf("raw/disruptions/%d/%02d/%02d/run_%d.parquet",
		dateFrom.Year(), dateFrom.Month(), dateFrom.Day(), runID)

	data := wrapAsParquetJSON(payload, map[string]string{
		"date_from":        dateFrom.Format("2006-01-02"),
		"date_to":          dateTo.Format("2006-01-02"),
		"record_count":     fmt.Sprintf("%d", recordCount),
		"ingestion_run_id": fmt.Sprintf("%d", runID),
		"fetched_at":       time.Now().UTC().Format(time.RFC3339),
	})

	if err := s.putObject(ctx, key, data); err != nil {
		return "", fmt.Errorf("put raw disruptions: %w", err)
	}

	return key, nil
}

func (s *Store) putObject(ctx context.Context, key string, data []byte) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String("application/octet-stream"),
	})
	if err != nil {
		return fmt.Errorf("put object %s: %w", key, err)
	}
	return nil
}

type rawPayloadEnvelope struct {
	Metadata map[string]string `json:"metadata"`
	Payload  json.RawMessage   `json:"payload"`
}

func wrapAsParquetJSON(payload []byte, metadata map[string]string) []byte {
	envelope := rawPayloadEnvelope{
		Metadata: metadata,
		Payload:  payload,
	}
	data, _ := json.Marshal(envelope)
	return data
}
