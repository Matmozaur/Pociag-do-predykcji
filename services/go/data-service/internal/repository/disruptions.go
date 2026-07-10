package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/pociag-do-predykcji/services/go/data-service/internal/model"
	"github.com/pociag-do-predykcji/services/go/data-service/internal/service"
)

func (r *Repository) QueryDisruptions(ctx context.Context, p service.QueryDisruptionsParams) ([]model.DisruptionSummary, int64, error) {
	ctx, span := r.tracer.Start(ctx, "db.disruptions.query")
	defer span.End()

	var conditions []string
	var params []any
	paramIdx := 1

	if p.DateFrom != nil {
		// Include disruptions active on or after dateFrom (date_to >= dateFrom or no end date).
		conditions = append(conditions, fmt.Sprintf("(d.date_to IS NULL OR d.date_to >= $%d::date)", paramIdx))
		params = append(params, p.DateFrom.Format("2006-01-02"))
		paramIdx++
	}
	if p.DateTo != nil {
		// Include disruptions starting on or before dateTo (date_from <= dateTo or no start date).
		conditions = append(conditions, fmt.Sprintf("(d.date_from IS NULL OR d.date_from <= $%d::date)", paramIdx))
		params = append(params, p.DateTo.Format("2006-01-02"))
		paramIdx++
	}
	if len(p.StationExternalIds) > 0 {
		conditions = append(conditions, fmt.Sprintf(
			"(d.start_station_ext_id = ANY($%d) OR d.end_station_ext_id = ANY($%d))",
			paramIdx, paramIdx,
		))
		params = append(params, intsToInt32s(p.StationExternalIds))
		paramIdx++
	}
	if p.TypeCode != "" {
		conditions = append(conditions, fmt.Sprintf("d.disruption_type_code = $%d", paramIdx))
		params = append(params, p.TypeCode)
		paramIdx++
	}

	params = append(params, p.Limit, p.Offset)
	limitOffsetClause := fmt.Sprintf("LIMIT $%d OFFSET $%d", paramIdx, paramIdx+1)

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	query := fmt.Sprintf(`
		SELECT
		    d.id, d.external_disruption_id,
		    d.disruption_type_code, dt.name AS disruption_type_name,
		    d.start_station_ext_id, s_start.name AS start_station_name,
		    d.end_station_ext_id,   s_end.name   AS end_station_name,
		    d.message, d.date_from, d.date_to,
		    COUNT(dar.id)  AS affected_routes_count,
		    COUNT(*) OVER() AS total_count
		FROM disruptions d
		LEFT JOIN disruption_types dt
		    ON dt.code = d.disruption_type_code
		LEFT JOIN stations s_start
		    ON s_start.external_id = d.start_station_ext_id
		LEFT JOIN stations s_end
		    ON s_end.external_id = d.end_station_ext_id
		LEFT JOIN disruption_affected_routes dar
		    ON dar.disruption_id = d.id
		%s
		GROUP BY d.id, dt.name, s_start.name, s_end.name
		ORDER BY d.date_from DESC NULLS LAST
		%s`, whereClause, limitOffsetClause)

	rows, err := r.db.WithContext(ctx).Raw(query, params...).Rows()
	if err != nil {
		return nil, 0, fmt.Errorf("query disruptions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []model.DisruptionSummary
	var total int64
	for rows.Next() {
		var (
			id                  int64
			externalID          int64
			typeCode            pgtype.Text
			typeName            pgtype.Text
			startStationExtID   pgtype.Int4
			startStationName    pgtype.Text
			endStationExtID     pgtype.Int4
			endStationName      pgtype.Text
			message             pgtype.Text
			dateFrom            pgtype.Date
			dateTo              pgtype.Date
			affectedRoutesCount int64
			totalCount          int64
		)
		if err := rows.Scan(
			&id, &externalID,
			&typeCode, &typeName,
			&startStationExtID, &startStationName,
			&endStationExtID, &endStationName,
			&message, &dateFrom, &dateTo,
			&affectedRoutesCount, &totalCount,
		); err != nil {
			return nil, 0, fmt.Errorf("scan disruption row: %w", err)
		}
		total = totalCount
		d := model.DisruptionSummary{
			ID:                   id,
			ExternalDisruptionID: externalID,
			AffectedRoutesCount:  int(affectedRoutesCount),
		}
		if typeCode.Valid {
			d.DisruptionTypeCode = &typeCode.String
		}
		if typeName.Valid {
			d.DisruptionTypeName = &typeName.String
		}
		if startStationExtID.Valid {
			v := int(startStationExtID.Int32)
			d.StartStationExtID = &v
		}
		if startStationName.Valid {
			d.StartStationName = &startStationName.String
		}
		if endStationExtID.Valid {
			v := int(endStationExtID.Int32)
			d.EndStationExtID = &v
		}
		if endStationName.Valid {
			d.EndStationName = &endStationName.String
		}
		if message.Valid {
			d.Message = &message.String
		}
		if dateFrom.Valid {
			s := dateFrom.Time.Format("2006-01-02")
			d.DateFrom = &s
		}
		if dateTo.Valid {
			s := dateTo.Time.Format("2006-01-02")
			d.DateTo = &s
		}
		results = append(results, d)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate disruption rows: %w", err)
	}
	if results == nil {
		results = []model.DisruptionSummary{}
	}
	return results, total, nil
}

func (r *Repository) GetDisruptionById(ctx context.Context, id int64) (*model.DisruptionDetail, error) {
	ctx, span := r.tracer.Start(ctx, "db.disruptions.get_by_id")
	defer span.End()

	const disruptionQuery = `
		SELECT
		    d.id, d.external_disruption_id,
		    d.disruption_type_code, dt.name AS disruption_type_name,
		    d.start_station_ext_id, s_start.name AS start_station_name,
		    d.end_station_ext_id,   s_end.name   AS end_station_name,
		    d.message, d.date_from, d.date_to
		FROM disruptions d
		LEFT JOIN disruption_types dt
		    ON dt.code = d.disruption_type_code
		LEFT JOIN stations s_start
		    ON s_start.external_id = d.start_station_ext_id
		LEFT JOIN stations s_end
		    ON s_end.external_id = d.end_station_ext_id
		WHERE d.id = $1`

	var (
		disruptionID      int64
		externalID        int64
		typeCode          pgtype.Text
		typeName          pgtype.Text
		startStationExtID pgtype.Int4
		startStationName  pgtype.Text
		endStationExtID   pgtype.Int4
		endStationName    pgtype.Text
		message           pgtype.Text
		dateFrom          pgtype.Date
		dateTo            pgtype.Date
	)
	err := r.db.WithContext(ctx).Raw(disruptionQuery, id).Row().Scan(
		&disruptionID, &externalID,
		&typeCode, &typeName,
		&startStationExtID, &startStationName,
		&endStationExtID, &endStationName,
		&message, &dateFrom, &dateTo,
	)
	if err != nil {
		if isNoRows(err) {
			return nil, fmt.Errorf("get disruption by id: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("get disruption by id: %w", err)
	}

	detail := &model.DisruptionDetail{
		ID:                   disruptionID,
		ExternalDisruptionID: externalID,
		AffectedRoutes:       []model.DisruptionAffectedRoute{},
	}
	if typeCode.Valid {
		detail.DisruptionTypeCode = &typeCode.String
	}
	if typeName.Valid {
		detail.DisruptionTypeName = &typeName.String
	}
	if startStationExtID.Valid {
		v := int(startStationExtID.Int32)
		detail.StartStationExtID = &v
	}
	if startStationName.Valid {
		detail.StartStationName = &startStationName.String
	}
	if endStationExtID.Valid {
		v := int(endStationExtID.Int32)
		detail.EndStationExtID = &v
	}
	if endStationName.Valid {
		detail.EndStationName = &endStationName.String
	}
	if message.Valid {
		detail.Message = &message.String
	}
	if dateFrom.Valid {
		s := dateFrom.Time.Format("2006-01-02")
		detail.DateFrom = &s
	}
	if dateTo.Valid {
		s := dateTo.Time.Format("2006-01-02")
		detail.DateTo = &s
	}

	// Query affected routes
	const routesQuery = `
		SELECT dar.schedule_id, dar.order_id, dar.train_order_id,
		       dar.operating_date, dar.station_ext_id,
		       s.name AS station_name, dar.sequence_number
		FROM disruption_affected_routes dar
		LEFT JOIN stations s ON s.external_id = dar.station_ext_id
		WHERE dar.disruption_id = $1
		ORDER BY dar.id`

	rows, err := r.db.WithContext(ctx).Raw(routesQuery, id).Rows()
	if err != nil {
		return nil, fmt.Errorf("query disruption affected routes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var (
			scheduleID    int32
			orderID       int32
			trainOrderID  pgtype.Int4
			operatingDate pgtype.Date
			stationExtID  pgtype.Int4
			stationName   pgtype.Text
			seqNumber     pgtype.Int4
		)
		if err := rows.Scan(
			&scheduleID, &orderID, &trainOrderID,
			&operatingDate, &stationExtID, &stationName, &seqNumber,
		); err != nil {
			return nil, fmt.Errorf("scan disruption affected route row: %w", err)
		}
		ar := model.DisruptionAffectedRoute{
			ScheduleID: int(scheduleID),
			OrderID:    int(orderID),
		}
		if trainOrderID.Valid {
			v := int(trainOrderID.Int32)
			ar.TrainOrderID = &v
		}
		if operatingDate.Valid {
			s := operatingDate.Time.Format("2006-01-02")
			ar.OperatingDate = &s
		}
		if stationExtID.Valid {
			v := int(stationExtID.Int32)
			ar.StationExtID = &v
		}
		if stationName.Valid {
			ar.StationName = &stationName.String
		}
		if seqNumber.Valid {
			v := int(seqNumber.Int32)
			ar.SequenceNumber = &v
		}
		detail.AffectedRoutes = append(detail.AffectedRoutes, ar)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate disruption affected route rows: %w", err)
	}
	return detail, nil
}
