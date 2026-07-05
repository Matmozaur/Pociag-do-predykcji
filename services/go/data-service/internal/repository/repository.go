package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

var ErrNotFound = errors.New("not found")

type Repository struct {
	db     *gorm.DB
	tracer trace.Tracer
}

func New(db *gorm.DB) *Repository {
	return &Repository{
		db:     db,
		tracer: otel.Tracer("pociag.data-service"),
	}
}

func (r *Repository) Ping(ctx context.Context) error {
	ctx, span := r.tracer.Start(ctx, "db.connection.ping")
	defer span.End()

	sqlDB, err := r.db.DB()
	if err != nil {
		return fmt.Errorf("get sql db: %w", err)
	}

	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("ping db: %w", err)
	}

	return nil
}

func isNoRows(err error) bool {
	return errors.Is(err, sql.ErrNoRows) || errors.Is(err, gorm.ErrRecordNotFound)
}

// intsToInt32s converts a []int slice to []int32 for use with PostgreSQL int4 array params.
func intsToInt32s(ints []int) []int32 {
	if len(ints) == 0 {
		return nil
	}
	result := make([]int32, len(ints))
	for i, v := range ints {
		result[i] = int32(v)
	}
	return result
}
