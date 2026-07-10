package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/pociag-do-predykcji/services/go/data-service/internal/model"
	"github.com/pociag-do-predykcji/services/go/data-service/internal/service"
)

func (r *Repository) QueryStations(ctx context.Context, p service.QueryStationsParams) ([]model.Station, int64, error) {
	ctx, span := r.tracer.Start(ctx, "db.stations.query")
	defer span.End()

	var conditions []string
	var params []any
	paramIdx := 1

	if p.Search != "" {
		conditions = append(conditions, fmt.Sprintf("s.name ILIKE $%d || '%%'", paramIdx))
		params = append(params, p.Search)
		paramIdx++
	}
	if p.City != "" {
		conditions = append(conditions, fmt.Sprintf("s.city = $%d", paramIdx))
		params = append(params, p.City)
		paramIdx++
	}
	if len(p.ExternalIds) > 0 {
		conditions = append(conditions, fmt.Sprintf("s.external_id = ANY($%d)", paramIdx))
		params = append(params, intsToInt32s(p.ExternalIds))
		paramIdx++
	}

	params = append(params, p.Limit, p.Offset)
	limitOffsetClause := fmt.Sprintf("LIMIT $%d OFFSET $%d", paramIdx, paramIdx+1)

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	query := fmt.Sprintf(`
		SELECT s.id, s.external_id, s.name, s.city, COUNT(*) OVER() AS total_count
		FROM stations s
		%s
		ORDER BY s.name
		%s`, whereClause, limitOffsetClause)

	rows, err := r.db.WithContext(ctx).Raw(query, params...).Rows()
	if err != nil {
		return nil, 0, fmt.Errorf("query stations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []model.Station
	var total int64
	for rows.Next() {
		var (
			id         int64
			externalID int32
			name       string
			city       pgtype.Text
			totalCount int64
		)
		if err := rows.Scan(&id, &externalID, &name, &city, &totalCount); err != nil {
			return nil, 0, fmt.Errorf("scan station row: %w", err)
		}
		total = totalCount
		st := model.Station{ID: id, ExternalID: int(externalID), Name: name}
		if city.Valid {
			st.City = &city.String
		}
		results = append(results, st)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate station rows: %w", err)
	}
	if results == nil {
		results = []model.Station{}
	}
	return results, total, nil
}

func (r *Repository) GetStationByExternalId(ctx context.Context, extID int) (*model.Station, error) {
	ctx, span := r.tracer.Start(ctx, "db.stations.get_by_external_id")
	defer span.End()

	const query = `
		SELECT id, external_id, name, city
		FROM stations
		WHERE external_id = $1`

	var (
		id         int64
		externalID int32
		name       string
		city       pgtype.Text
	)
	err := r.db.WithContext(ctx).Raw(query, int32(extID)).Row().Scan(&id, &externalID, &name, &city)
	if err != nil {
		if isNoRows(err) {
			return nil, fmt.Errorf("get station by external id: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("get station by external id: %w", err)
	}

	st := &model.Station{ID: id, ExternalID: int(externalID), Name: name}
	if city.Valid {
		st.City = &city.String
	}
	return st, nil
}

func (r *Repository) ListCarriers(ctx context.Context) ([]model.Carrier, error) {
	ctx, span := r.tracer.Start(ctx, "db.carriers.list")
	defer span.End()

	const query = `
		SELECT id, code, name, valid_from, valid_to
		FROM carriers
		ORDER BY code`

	rows, err := r.db.WithContext(ctx).Raw(query).Rows()
	if err != nil {
		return nil, fmt.Errorf("list carriers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []model.Carrier
	for rows.Next() {
		var (
			id        int64
			code      string
			name      string
			validFrom pgtype.Timestamptz
			validTo   pgtype.Timestamptz
		)
		if err := rows.Scan(&id, &code, &name, &validFrom, &validTo); err != nil {
			return nil, fmt.Errorf("scan carrier row: %w", err)
		}
		c := model.Carrier{ID: id, Code: code, Name: name}
		if validFrom.Valid {
			c.ValidFrom = &validFrom.Time
		}
		if validTo.Valid {
			c.ValidTo = &validTo.Time
		}
		results = append(results, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate carrier rows: %w", err)
	}
	if results == nil {
		results = []model.Carrier{}
	}
	return results, nil
}

func (r *Repository) ListCommercialCategories(ctx context.Context) ([]model.CommercialCategory, error) {
	ctx, span := r.tracer.Start(ctx, "db.commercial_categories.list")
	defer span.End()

	const query = `
		SELECT id, code, name, carrier_code, speed_category_code
		FROM commercial_categories
		ORDER BY code`

	rows, err := r.db.WithContext(ctx).Raw(query).Rows()
	if err != nil {
		return nil, fmt.Errorf("list commercial categories: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []model.CommercialCategory
	for rows.Next() {
		var (
			id                int64
			code              string
			name              string
			carrierCode       pgtype.Text
			speedCategoryCode pgtype.Text
		)
		if err := rows.Scan(&id, &code, &name, &carrierCode, &speedCategoryCode); err != nil {
			return nil, fmt.Errorf("scan commercial category row: %w", err)
		}
		cc := model.CommercialCategory{ID: id, Code: code, Name: name}
		if carrierCode.Valid {
			cc.CarrierCode = &carrierCode.String
		}
		if speedCategoryCode.Valid {
			cc.SpeedCategoryCode = &speedCategoryCode.String
		}
		results = append(results, cc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate commercial category rows: %w", err)
	}
	if results == nil {
		results = []model.CommercialCategory{}
	}
	return results, nil
}

func (r *Repository) ListStopTypes(ctx context.Context) ([]model.StopType, error) {
	ctx, span := r.tracer.Start(ctx, "db.stop_types.list")
	defer span.End()

	const query = `
		SELECT id, external_id, description
		FROM stop_types
		ORDER BY external_id`

	rows, err := r.db.WithContext(ctx).Raw(query).Rows()
	if err != nil {
		return nil, fmt.Errorf("list stop types: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []model.StopType
	for rows.Next() {
		var (
			id          int64
			externalID  int32
			description string
		)
		if err := rows.Scan(&id, &externalID, &description); err != nil {
			return nil, fmt.Errorf("scan stop type row: %w", err)
		}
		results = append(results, model.StopType{ID: id, ExternalID: int(externalID), Description: description})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stop type rows: %w", err)
	}
	if results == nil {
		results = []model.StopType{}
	}
	return results, nil
}
