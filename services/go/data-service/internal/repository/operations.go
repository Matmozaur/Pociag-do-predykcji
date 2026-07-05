package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/pociag-do-predykcji/services/go/data-service/internal/model"
	"github.com/pociag-do-predykcji/services/go/data-service/internal/service"
)

func (r *Repository) QueryOperations(ctx context.Context, p service.QueryOperationsParams) ([]model.OperationSummary, int64, error) {
	ctx, span := r.tracer.Start(ctx, "db.train_operations.query")
	defer span.End()

	var conditions []string
	var params []any
	paramIdx := 1

	if p.Date != nil {
		conditions = append(conditions, fmt.Sprintf("to2.operating_date = $%d::date", paramIdx))
		params = append(params, p.Date.Format("2006-01-02"))
		paramIdx++
	}
	if len(p.StationExternalIds) > 0 {
		conditions = append(conditions, fmt.Sprintf(
			"EXISTS (SELECT 1 FROM operation_stations os2 WHERE os2.train_operation_id = to2.id AND os2.station_external_id = ANY($%d))",
			paramIdx,
		))
		params = append(params, intsToInt32s(p.StationExternalIds))
		paramIdx++
	}
	if p.Status != "" {
		conditions = append(conditions, fmt.Sprintf("to2.train_status = $%d", paramIdx))
		params = append(params, p.Status)
		paramIdx++
	}
	if len(p.CarrierCodes) > 0 {
		conditions = append(conditions, fmt.Sprintf("r.carrier_code = ANY($%d)", paramIdx))
		params = append(params, p.CarrierCodes)
		paramIdx++
	}
	if p.MinDelay != nil {
		conditions = append(conditions, fmt.Sprintf(
			"(SELECT GREATEST(COALESCE(MAX(os3.arrival_delay_minutes), 0), COALESCE(MAX(os3.departure_delay_minutes), 0)) FROM operation_stations os3 WHERE os3.train_operation_id = to2.id) >= $%d",
			paramIdx,
		))
		params = append(params, int32(*p.MinDelay))
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
		    to2.id, to2.schedule_id, to2.order_id, to2.train_order_id,
		    to2.operating_date, to2.train_status,
		    MAX(r.name)         AS route_name,
		    MAX(r.carrier_code) AS carrier_code,
		    COUNT(os.id)        AS station_count,
		    MAX(os.arrival_delay_minutes)   AS max_arrival_delay_minutes,
		    MAX(os.departure_delay_minutes) AS max_departure_delay_minutes,
		    COUNT(*) OVER()     AS total_count
		FROM train_operations to2
		LEFT JOIN routes r
		    ON r.schedule_id = to2.schedule_id AND r.order_id = to2.order_id
		LEFT JOIN operation_stations os
		    ON os.train_operation_id = to2.id
		%s
		GROUP BY to2.id, to2.schedule_id, to2.order_id, to2.train_order_id,
		         to2.operating_date, to2.train_status
		ORDER BY to2.id
		%s`, whereClause, limitOffsetClause)

	rows, err := r.db.WithContext(ctx).Raw(query, params...).Rows()
	if err != nil {
		return nil, 0, fmt.Errorf("query operations: %w", err)
	}
	defer rows.Close()

	var results []model.OperationSummary
	var total int64
	for rows.Next() {
		var (
			id              int64
			scheduleID      int32
			orderID         int32
			trainOrderID    pgtype.Int4
			operatingDate   pgtype.Date
			trainStatus     string
			routeName       pgtype.Text
			carrierCode     pgtype.Text
			stationCount    int64
			maxArrivalDelay pgtype.Int4
			maxDepartDelay  pgtype.Int4
			totalCount      int64
		)
		if err := rows.Scan(
			&id, &scheduleID, &orderID, &trainOrderID,
			&operatingDate, &trainStatus,
			&routeName, &carrierCode,
			&stationCount, &maxArrivalDelay, &maxDepartDelay,
			&totalCount,
		); err != nil {
			return nil, 0, fmt.Errorf("scan operation row: %w", err)
		}
		total = totalCount
		op := model.OperationSummary{
			ID:           id,
			ScheduleID:   int(scheduleID),
			OrderID:      int(orderID),
			TrainStatus:  trainStatus,
			StationCount: int(stationCount),
		}
		if operatingDate.Valid {
			op.OperatingDate = operatingDate.Time.Format("2006-01-02")
		}
		if trainOrderID.Valid {
			v := int(trainOrderID.Int32)
			op.TrainOrderID = &v
		}
		if routeName.Valid {
			op.RouteName = &routeName.String
		}
		if carrierCode.Valid {
			op.CarrierCode = &carrierCode.String
		}
		if maxArrivalDelay.Valid {
			v := int(maxArrivalDelay.Int32)
			op.MaxArrivalDelayMinutes = &v
		}
		if maxDepartDelay.Valid {
			v := int(maxDepartDelay.Int32)
			op.MaxDepartureDelayMinutes = &v
		}
		results = append(results, op)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate operation rows: %w", err)
	}
	if results == nil {
		results = []model.OperationSummary{}
	}
	return results, total, nil
}

func (r *Repository) GetOperationById(ctx context.Context, id int64) (*model.OperationDetail, error) {
	ctx, span := r.tracer.Start(ctx, "db.train_operations.get_by_id")
	defer span.End()

	const opQuery = `
		SELECT to2.id, to2.schedule_id, to2.order_id, to2.train_order_id,
		       to2.operating_date, to2.train_status,
		       MAX(r.name)         AS route_name,
		       MAX(r.carrier_code) AS carrier_code
		FROM train_operations to2
		LEFT JOIN routes r
		    ON r.schedule_id = to2.schedule_id AND r.order_id = to2.order_id
		WHERE to2.id = $1
		GROUP BY to2.id, to2.schedule_id, to2.order_id, to2.train_order_id,
		         to2.operating_date, to2.train_status`

	var (
		opID          int64
		scheduleID    int32
		orderID       int32
		trainOrderID  pgtype.Int4
		operatingDate pgtype.Date
		trainStatus   string
		routeName     pgtype.Text
		carrierCode   pgtype.Text
	)
	err := r.db.WithContext(ctx).Raw(opQuery, id).Row().Scan(
		&opID, &scheduleID, &orderID, &trainOrderID,
		&operatingDate, &trainStatus, &routeName, &carrierCode,
	)
	if err != nil {
		if isNoRows(err) {
			return nil, fmt.Errorf("get operation by id: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("get operation by id: %w", err)
	}

	detail := &model.OperationDetail{
		ID:          opID,
		ScheduleID:  int(scheduleID),
		OrderID:     int(orderID),
		TrainStatus: trainStatus,
	}
	if operatingDate.Valid {
		detail.OperatingDate = operatingDate.Time.Format("2006-01-02")
	}
	if trainOrderID.Valid {
		v := int(trainOrderID.Int32)
		detail.TrainOrderID = &v
	}
	if routeName.Valid {
		detail.RouteName = &routeName.String
	}
	if carrierCode.Valid {
		detail.CarrierCode = &carrierCode.String
	}

	// Query stations
	const stationsQuery = `
		SELECT os.station_external_id, s.name,
		       os.planned_sequence_number, os.actual_sequence_number,
		       os.planned_arrival, os.planned_departure,
		       os.actual_arrival, os.actual_departure,
		       os.arrival_delay_minutes, os.departure_delay_minutes,
		       os.is_confirmed, os.is_cancelled
		FROM operation_stations os
		LEFT JOIN stations s ON s.external_id = os.station_external_id
		WHERE os.train_operation_id = $1
		ORDER BY os.actual_sequence_number`

	rows, err := r.db.WithContext(ctx).Raw(stationsQuery, id).Rows()
	if err != nil {
		return nil, fmt.Errorf("query operation stations: %w", err)
	}
	defer rows.Close()

	detail.Stations = []model.OperationStation{}
	for rows.Next() {
		var (
			stationExtID     int32
			stationName      pgtype.Text
			plannedSeq       pgtype.Int4
			actualSeq        int32
			plannedArrival   pgtype.Timestamptz
			plannedDeparture pgtype.Timestamptz
			actualArrival    pgtype.Timestamptz
			actualDeparture  pgtype.Timestamptz
			arrivalDelay     pgtype.Int4
			departureDelay   pgtype.Int4
			isConfirmed      bool
			isCancelled      bool
		)
		if err := rows.Scan(
			&stationExtID, &stationName,
			&plannedSeq, &actualSeq,
			&plannedArrival, &plannedDeparture,
			&actualArrival, &actualDeparture,
			&arrivalDelay, &departureDelay,
			&isConfirmed, &isCancelled,
		); err != nil {
			return nil, fmt.Errorf("scan operation station row: %w", err)
		}
		st := model.OperationStation{
			StationExternalID:    int(stationExtID),
			ActualSequenceNumber: int(actualSeq),
			IsConfirmed:          isConfirmed,
			IsCancelled:          isCancelled,
		}
		if stationName.Valid {
			st.StationName = &stationName.String
		}
		if plannedSeq.Valid {
			v := int(plannedSeq.Int32)
			st.PlannedSequenceNumber = &v
		}
		if plannedArrival.Valid {
			st.PlannedArrival = &plannedArrival.Time
		}
		if plannedDeparture.Valid {
			st.PlannedDeparture = &plannedDeparture.Time
		}
		if actualArrival.Valid {
			st.ActualArrival = &actualArrival.Time
		}
		if actualDeparture.Valid {
			st.ActualDeparture = &actualDeparture.Time
		}
		if arrivalDelay.Valid {
			v := int(arrivalDelay.Int32)
			st.ArrivalDelayMinutes = &v
		}
		if departureDelay.Valid {
			v := int(departureDelay.Int32)
			st.DepartureDelayMinutes = &v
		}
		detail.Stations = append(detail.Stations, st)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate operation station rows: %w", err)
	}
	return detail, nil
}

func (r *Repository) GetOperationStatistics(ctx context.Context, date time.Time) (*model.OperationStatistics, error) {
	ctx, span := r.tracer.Start(ctx, "db.train_operations.statistics")
	defer span.End()

	const query = `
		WITH op_delays AS (
		    SELECT
		        to2.train_status,
		        GREATEST(
		            COALESCE(MAX(os.arrival_delay_minutes), 0),
		            COALESCE(MAX(os.departure_delay_minutes), 0)
		        ) AS max_delay
		    FROM train_operations to2
		    LEFT JOIN operation_stations os ON os.train_operation_id = to2.id
		    WHERE to2.operating_date = $1::date
		    GROUP BY to2.id, to2.train_status
		)
		SELECT
		    COUNT(*)                                         AS total,
		    COUNT(*) FILTER (WHERE train_status = 'S')      AS status_s,
		    COUNT(*) FILTER (WHERE train_status = 'P')      AS status_p,
		    COUNT(*) FILTER (WHERE train_status = 'C')      AS status_c,
		    COUNT(*) FILTER (WHERE train_status = 'X')      AS status_x,
		    COUNT(*) FILTER (WHERE train_status = 'Q')      AS status_q,
		    COUNT(*) FILTER (WHERE max_delay <= 5)          AS on_time,
		    COUNT(*) FILTER (WHERE max_delay > 5  AND max_delay <= 15) AS slight_delay,
		    COUNT(*) FILTER (WHERE max_delay > 15 AND max_delay <= 60) AS moderate_delay,
		    COUNT(*) FILTER (WHERE max_delay > 60)          AS severe_delay,
		    AVG(CASE WHEN max_delay > 0 THEN max_delay::float8 END) AS avg_delay
		FROM op_delays`

	var (
		total         int64
		statusS       int64
		statusP       int64
		statusC       int64
		statusX       int64
		statusQ       int64
		onTime        int64
		slightDelay   int64
		moderateDelay int64
		severeDelay   int64
		avgDelay      pgtype.Float8
	)
	dateStr := date.Format("2006-01-02")
	err := r.db.WithContext(ctx).Raw(query, dateStr).Row().Scan(
		&total, &statusS, &statusP, &statusC, &statusX, &statusQ,
		&onTime, &slightDelay, &moderateDelay, &severeDelay, &avgDelay,
	)
	if err != nil {
		return nil, fmt.Errorf("get operation statistics: %w", err)
	}

	byStatus := make(map[string]int)
	for code, count := range map[string]int64{
		"S": statusS,
		"P": statusP,
		"C": statusC,
		"X": statusX,
		"Q": statusQ,
	} {
		if count > 0 {
			byStatus[code] = int(count)
		}
	}

	stats := &model.OperationStatistics{
		Date:     date.Format("2006-01-02"),
		Total:    int(total),
		ByStatus: byStatus,
		DelayDistribution: model.DelayDistribution{
			OnTime:        int(onTime),
			SlightDelay:   int(slightDelay),
			ModerateDelay: int(moderateDelay),
			SevereDelay:   int(severeDelay),
		},
	}
	if avgDelay.Valid {
		stats.AvgDelayMinutes = &avgDelay.Float64
	}
	return stats, nil
}
