package main

import (
	"context"
	"fmt"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/pociag-do-predykcji/services/go/data-service/internal/config"
	"github.com/pociag-do-predykcji/services/go/data-service/internal/handler"
	"github.com/pociag-do-predykcji/services/go/data-service/internal/repository"
	"github.com/pociag-do-predykcji/services/go/data-service/internal/service"
)

// @title			Pociag Data Service API
// @version		0.1.0
// @description	Internal domain read service. Exposes curated schedules, operations, disruptions, and dictionaries.
// @host			localhost:8083
// @basePath		/
// @schemes		http https
// @x-internal	true
func main() {
	logger, err := zap.NewProduction()
	if err != nil {
		panic(fmt.Sprintf("init logger: %v", err))
	}
	defer func() {
		_ = logger.Sync()
	}()

	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("load config", zap.Error(err))
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	shutdownTracer, err := setupTracing(ctx, cfg.OTLPExporterEndpoint)
	if err != nil {
		logger.Fatal("setup tracing", zap.Error(err))
	}
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if shutdownErr := shutdownTracer(shutdownCtx); shutdownErr != nil {
			logger.Error("shutdown tracer", zap.Error(shutdownErr))
		}
	}()

	db, err := gorm.Open(postgres.Open(cfg.DatabaseDSN), &gorm.Config{})
	if err != nil {
		logger.Fatal("connect db", zap.Error(err))
	}

	repo := repository.New(db)
	svc := service.New(repo)
	h := handler.New(svc)

	r := chi.NewRouter()
	r.Use(otelhttp.NewMiddleware("pociag.data-service"))
	r.Handle("/metrics", promhttp.Handler())
	h.RegisterRoutes(r)
	h.RegisterDocsRoutes(r)

	server := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info("starting data-service", zap.String("addr", cfg.HTTPAddr))
		if serveErr := server.ListenAndServe(); serveErr != nil && serveErr != http.ErrServerClosed {
			logger.Fatal("listen and serve", zap.Error(serveErr))
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down data-service")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown server", zap.Error(err))
	}
}

func setupTracing(ctx context.Context, endpoint string) (func(context.Context) error, error) {
	exporter, err := otlptracegrpc.New(
		ctx,
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("create otlp exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String("pociag.data-service"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create trace resource: %w", err)
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	return provider.Shutdown, nil
}
