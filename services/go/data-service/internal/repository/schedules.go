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

// intervalToTimeStr converts a pgtype.Interval to an "HH:MM" string pointer.
// Returns nil if the interval is not valid.
func intervalToTimeStr(iv pgtype.Interval) *string {
	if !iv.Valid {
		return nil
	}
	totalSecs := iv.Microseconds/1_000_000 + int64(iv.Days)*86400
	hours := totalSecs / 3600
	mins := (totalSecs % 3600) / 60
	s := fmt.Sprintf("%02d:%02d", hours, mins)
	return &s
}

// scanRouteStations executes a query to retrieve route stations joined with the stations table.
// The query must return columns in the order:
// station_external_id, station_name, order_number, arrival_time, departure_time,
// arrival_day, departure_day, platform, track, stop_type_name, commercial_category
func scanRouteStations(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}) ([]model.RouteStation, error) {
	var results []model.RouteStation
	for rows.Next() {
		var (
			stationExternalID int32
			stationName       pgtype.Text
			orderNumber       int32
			arrivalTime       pgtype.Interval
			departureTime     pgtype.Interval
			arrivalDay        pgtype.Int4
			departureDay      pgtype.Int4
			platform          pgtype.Text
			track             pgtype.Text
			stopType          pgtype.Text
			commCategory      pgtype.Text
		)
		if err := rows.Scan(
			&stationExternalID, &stationName, &orderNumber,
			&arrivalTime, &departureTime,
			&arrivalDay, &departureDay,
			&platform, &track, &stopType, &commCategory,
		); err != nil {
			return nil, fmt.Errorf("scan route station row: %w", err)
		}
		rs := model.RouteStation{
			StationExternalID: int(stationExternalID),
			OrderNumber:       int(orderNumber),
		}
		if stationName.Valid {
			rs.StationName = &stationName.String
		}
		rs.ArrivalTime = intervalToTimeStr(arrivalTime)
		rs.DepartureTime = intervalToTimeStr(departureTime)
		if arrivalDay.Valid {
			v := int(arrivalDay.Int32)
			rs.ArrivalDay = &v
		}
		if departureDay.Valid {
			v := int(departureDay.Int32)
			rs.DepartureDay = &v
		}
		if platform.Valid {
			rs.Platform = &platform.String
		}
		if track.Valid {
			rs.Track = &track.String
		}
		if stopType.Valid {
			rs.StopType = &stopType.String
		}
		if commCategory.Valid {
			rs.CommercialCategory = &commCategory.String
		}
		results = append(results, rs)
	}
	if results == nil {
		results = []model.RouteStation{}
	}
	return results, nil
}

func (r *Repository) QueryRoutes(ctx context.Context, p service.QueryRoutesParams) ([]model.RouteSummary, int64, error) {
	ctx, span := r.tracer.Start(ctx, "db.routes.query")
	defer span.End()

	var conditions []string
	var params []any
	paramIdx := 1

	// Date range conditions — also used inside the operating_dates subquery.
	dateCTEWhere := ""
	if p.DateFrom != nil || p.DateTo != nil {
		var dateConds []string
		if p.DateFrom != nil {
			dateConds = append(dateConds, fmt.Sprintf("rod.operating_date >= $%d::date", paramIdx))
			params = append(params, p.DateFrom.Format("2006-01-02"))
			paramIdx++
		}
		if p.DateTo != nil {
			dateConds = append(dateConds, fmt.Sprintf("rod.operating_date <= $%d::date", paramIdx))
			params = append(params, p.DateTo.Format("2006-01-02"))
			paramIdx++
		}
		dateCTEWhere = "WHERE " + strings.Join(dateConds, " AND ")
		// Routes must have at least one operating date in range.
		conditions = append(conditions, "dc.route_id IS NOT NULL")
	}

	if len(p.CarrierCodes) > 0 {
		conditions = append(conditions, fmt.Sprintf("r.carrier_code = ANY($%d)", paramIdx))
		params = append(params, p.CarrierCodes)
		paramIdx++
	}
	if p.CommercialCategory != "" {
		conditions = append(conditions, fmt.Sprintf("r.commercial_category_symbol = $%d", paramIdx))
		params = append(params, p.CommercialCategory)
		paramIdx++
	}
	if p.Name != "" {
		conditions = append(conditions, fmt.Sprintf("r.name ILIKE '%%' || $%d || '%%'", paramIdx))
		params = append(params, p.Name)
		paramIdx++
	}
	if len(p.StationExternalIds) > 0 {
		conditions = append(conditions, fmt.Sprintf(
			"EXISTS (SELECT 1 FROM route_stations rs2 WHERE rs2.route_id = r.id AND rs2.station_external_id = ANY($%d))",
			paramIdx,
		))
		params = append(params, intsToInt32s(p.StationExternalIds))
		paramIdx++
	}
	if p.FromCity != "" {
		conditions = append(conditions, fmt.Sprintf(
			"EXISTS (SELECT 1 FROM route_stations rsa JOIN stations sa ON sa.external_id = rsa.station_external_id WHERE rsa.route_id = r.id AND rsa.order_number = bounds.first_ord AND sa.city = $%d)",
			paramIdx,
		))
		params = append(params, p.FromCity)
		paramIdx++
	}
	if p.ToCity != "" {
		conditions = append(conditions, fmt.Sprintf(
			"EXISTS (SELECT 1 FROM route_stations rsb JOIN stations sb ON sb.external_id = rsb.station_external_id WHERE rsb.route_id = r.id AND rsb.order_number = bounds.last_ord AND sb.city = $%d)",
			paramIdx,
		))
		params = append(params, p.ToCity)
		paramIdx++
	}

	params = append(params, p.Limit, p.Offset)
	limitOffsetClause := fmt.Sprintf("LIMIT $%d OFFSET $%d", paramIdx, paramIdx+1)

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	var sb strings.Builder
	sb.WriteString(`
		SELECT r.id, r.schedule_id, r.order_id, r.train_order_id,
		       r.name, r.carrier_code, r.national_number, r.commercial_category_symbol,
		       s_first.name AS first_station_name,
		       s_last.name  AS last_station_name,
		       rs_first.departure_time AS first_departure_time,
		       rs_last.arrival_time    AS last_arrival_time,
		       bounds.station_count,
		       COALESCE(dc.cnt, 0) AS operating_dates_count,
		       COUNT(*) OVER() AS total_count
		FROM routes r
		JOIN (
		    SELECT route_id,
		           COUNT(*) AS station_count,
		           MIN(order_number) AS first_ord,
		           MAX(order_number) AS last_ord
		    FROM route_stations GROUP BY route_id
		) bounds ON bounds.route_id = r.id
		JOIN route_stations rs_first
		    ON rs_first.route_id = r.id AND rs_first.order_number = bounds.first_ord
		JOIN route_stations rs_last
		    ON rs_last.route_id = r.id AND rs_last.order_number = bounds.last_ord
		LEFT JOIN stations s_first ON s_first.external_id = rs_first.station_external_id
		LEFT JOIN stations s_last  ON s_last.external_id  = rs_last.station_external_id
		LEFT JOIN (
		    SELECT route_id, COUNT(*) AS cnt
		    FROM route_operating_dates rod
		    `)
	sb.WriteString(dateCTEWhere)
	sb.WriteString(`
		    GROUP BY route_id
		) dc ON dc.route_id = r.id
		`)
	sb.WriteString(whereClause)
	sb.WriteString(" ORDER BY r.id ")
	sb.WriteString(limitOffsetClause)

	rows, err := r.db.WithContext(ctx).Raw(sb.String(), params...).Rows()
	if err != nil {
		return nil, 0, fmt.Errorf("query routes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []model.RouteSummary
	var total int64
	for rows.Next() {
		var (
			id                       int64
			scheduleID               int32
			orderID                  int32
			trainOrderID             pgtype.Int4
			name                     pgtype.Text
			carrierCode              pgtype.Text
			nationalNumber           pgtype.Text
			commercialCategorySymbol pgtype.Text
			firstStationName         pgtype.Text
			lastStationName          pgtype.Text
			firstDepartureTime       pgtype.Interval
			lastArrivalTime          pgtype.Interval
			stationCount             int64
			operatingDatesCount      int64
			totalCount               int64
		)
		if err := rows.Scan(
			&id, &scheduleID, &orderID, &trainOrderID,
			&name, &carrierCode, &nationalNumber, &commercialCategorySymbol,
			&firstStationName, &lastStationName,
			&firstDepartureTime, &lastArrivalTime,
			&stationCount, &operatingDatesCount, &totalCount,
		); err != nil {
			return nil, 0, fmt.Errorf("scan route row: %w", err)
		}
		total = totalCount
		row := model.RouteSummary{
			ID:                  id,
			ScheduleID:          int(scheduleID),
			OrderID:             int(orderID),
			StationCount:        int(stationCount),
			OperatingDatesCount: int(operatingDatesCount),
		}
		if trainOrderID.Valid {
			v := int(trainOrderID.Int32)
			row.TrainOrderID = &v
		}
		if name.Valid {
			row.Name = &name.String
		}
		if carrierCode.Valid {
			row.CarrierCode = &carrierCode.String
		}
		if nationalNumber.Valid {
			row.NationalNumber = &nationalNumber.String
		}
		if commercialCategorySymbol.Valid {
			row.CommercialCategorySymbol = &commercialCategorySymbol.String
		}
		if firstStationName.Valid {
			row.FirstStationName = &firstStationName.String
		}
		if lastStationName.Valid {
			row.LastStationName = &lastStationName.String
		}
		row.FirstDepartureTime = intervalToTimeStr(firstDepartureTime)
		row.LastArrivalTime = intervalToTimeStr(lastArrivalTime)
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate route rows: %w", err)
	}
	if results == nil {
		results = []model.RouteSummary{}
	}
	return results, total, nil
}

func (r *Repository) GetRouteById(ctx context.Context, id int64) (*model.RouteDetail, error) {
	ctx, span := r.tracer.Start(ctx, "db.routes.get_by_id")
	defer span.End()

	detail, err := r.fetchRouteDetail(ctx, "WHERE r.id = $1", id)
	if err != nil {
		return nil, fmt.Errorf("get route by id: %w", err)
	}
	return detail, nil
}

func (r *Repository) GetRouteByKey(ctx context.Context, scheduleID, orderID int) (*model.RouteDetail, error) {
	ctx, span := r.tracer.Start(ctx, "db.routes.get_by_key")
	defer span.End()

	detail, err := r.fetchRouteDetail(ctx, "WHERE r.schedule_id = $1 AND r.order_id = $2", int32(scheduleID), int32(orderID))
	if err != nil {
		return nil, fmt.Errorf("get route by key: %w", err)
	}
	return detail, nil
}

// fetchRouteDetail loads a route with its stations, operating dates, and connections
// using the provided WHERE clause and params.
func (r *Repository) fetchRouteDetail(ctx context.Context, whereClause string, params ...any) (*model.RouteDetail, error) {
	routeQuery := fmt.Sprintf(`
		SELECT r.id, r.schedule_id, r.order_id, r.train_order_id,
		       r.name, r.carrier_code, r.national_number,
		       r.international_arrival_num, r.international_departure_num,
		       r.commercial_category_symbol
		FROM routes r
		%s`, whereClause)

	var (
		routeID                  int64
		scheduleID               int32
		orderID                  int32
		trainOrderID             pgtype.Int4
		name                     pgtype.Text
		carrierCode              pgtype.Text
		nationalNumber           pgtype.Text
		intlArrivalNum           pgtype.Text
		intlDepartureNum         pgtype.Text
		commercialCategorySymbol pgtype.Text
	)
	err := r.db.WithContext(ctx).Raw(routeQuery, params...).Row().Scan(
		&routeID, &scheduleID, &orderID, &trainOrderID,
		&name, &carrierCode, &nationalNumber,
		&intlArrivalNum, &intlDepartureNum,
		&commercialCategorySymbol,
	)
	if err != nil {
		if isNoRows(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("query route: %w", err)
	}

	detail := &model.RouteDetail{
		ID:         routeID,
		ScheduleID: int(scheduleID),
		OrderID:    int(orderID),
	}
	if trainOrderID.Valid {
		v := int(trainOrderID.Int32)
		detail.TrainOrderID = &v
	}
	if name.Valid {
		detail.Name = &name.String
	}
	if carrierCode.Valid {
		detail.CarrierCode = &carrierCode.String
	}
	if nationalNumber.Valid {
		detail.NationalNumber = &nationalNumber.String
	}
	if intlArrivalNum.Valid {
		detail.InternationalArrivalNum = &intlArrivalNum.String
	}
	if intlDepartureNum.Valid {
		detail.InternationalDepartureNum = &intlDepartureNum.String
	}
	if commercialCategorySymbol.Valid {
		detail.CommercialCategorySymbol = &commercialCategorySymbol.String
	}

	// Query 2: stations
	const stationsQuery = `
		SELECT rs.station_external_id, s.name, rs.order_number,
		       rs.arrival_time, rs.departure_time,
		       rs.arrival_day, rs.departure_day,
		       COALESCE(rs.departure_platform, rs.arrival_platform),
		       COALESCE(rs.departure_track, rs.arrival_track),
		       rs.stop_type_name,
		       rs.departure_commercial_category
		FROM route_stations rs
		LEFT JOIN stations s ON s.external_id = rs.station_external_id
		WHERE rs.route_id = $1
		ORDER BY rs.order_number`

	stRows, err := r.db.WithContext(ctx).Raw(stationsQuery, routeID).Rows()
	if err != nil {
		return nil, fmt.Errorf("query route stations: %w", err)
	}
	defer func() { _ = stRows.Close() }()

	detail.Stations, err = scanRouteStations(stRows)
	if err != nil {
		return nil, err
	}
	if err := stRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate route station rows: %w", err)
	}

	// Query 3: operating dates
	const datesQuery = `
		SELECT operating_date
		FROM route_operating_dates
		WHERE route_id = $1
		ORDER BY operating_date`

	dateRows, err := r.db.WithContext(ctx).Raw(datesQuery, routeID).Rows()
	if err != nil {
		return nil, fmt.Errorf("query route operating dates: %w", err)
	}
	defer func() { _ = dateRows.Close() }()

	detail.OperatingDates = []string{}
	for dateRows.Next() {
		var d pgtype.Date
		if err := dateRows.Scan(&d); err != nil {
			return nil, fmt.Errorf("scan operating date: %w", err)
		}
		if d.Valid {
			detail.OperatingDates = append(detail.OperatingDates, d.Time.Format("2006-01-02"))
		}
	}
	if err := dateRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate operating date rows: %w", err)
	}

	// Query 4: connections
	const connectionsQuery = `
		SELECT rc.external_connection_id, rc.type_code, rc.type_name,
		       rc.station_external_id, s.name,
		       rc.wagon_numbers, rc.train1_order_id, rc.train2_order_id
		FROM route_connections rc
		LEFT JOIN stations s ON s.external_id = rc.station_external_id
		WHERE rc.route_id = $1
		ORDER BY rc.id`

	connRows, err := r.db.WithContext(ctx).Raw(connectionsQuery, routeID).Rows()
	if err != nil {
		return nil, fmt.Errorf("query route connections: %w", err)
	}
	defer func() { _ = connRows.Close() }()

	for connRows.Next() {
		var (
			externalConnectionID pgtype.Text
			typeCode             string
			typeName             pgtype.Text
			stationExternalID    int32
			stationName          pgtype.Text
			wagonNumbers         pgtype.Text
			train1OrderID        pgtype.Int4
			train2OrderID        pgtype.Int4
		)
		if err := connRows.Scan(
			&externalConnectionID, &typeCode, &typeName,
			&stationExternalID, &stationName,
			&wagonNumbers, &train1OrderID, &train2OrderID,
		); err != nil {
			return nil, fmt.Errorf("scan route connection row: %w", err)
		}
		conn := model.RouteConnection{
			TypeCode:          typeCode,
			StationExternalID: int(stationExternalID),
		}
		if externalConnectionID.Valid {
			conn.ExternalConnectionID = &externalConnectionID.String
		}
		if typeName.Valid {
			conn.TypeName = &typeName.String
		}
		if stationName.Valid {
			conn.StationName = &stationName.String
		}
		if wagonNumbers.Valid {
			conn.WagonNumbers = &wagonNumbers.String
		}
		if train1OrderID.Valid {
			v := int(train1OrderID.Int32)
			conn.Train1OrderID = &v
		}
		if train2OrderID.Valid {
			v := int(train2OrderID.Int32)
			conn.Train2OrderID = &v
		}
		detail.Connections = append(detail.Connections, conn)
	}
	if err := connRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate route connection rows: %w", err)
	}

	return detail, nil
}

func (r *Repository) GetRouteStations(ctx context.Context, routeID int64) ([]model.RouteStation, error) {
	ctx, span := r.tracer.Start(ctx, "db.route_stations.list")
	defer span.End()

	// Verify route exists.
	var exists bool
	if err := r.db.WithContext(ctx).Raw("SELECT EXISTS(SELECT 1 FROM routes WHERE id = $1)", routeID).Row().Scan(&exists); err != nil {
		return nil, fmt.Errorf("check route exists: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("get route stations: %w", ErrNotFound)
	}

	const query = `
		SELECT rs.station_external_id, s.name, rs.order_number,
		       rs.arrival_time, rs.departure_time,
		       rs.arrival_day, rs.departure_day,
		       COALESCE(rs.departure_platform, rs.arrival_platform),
		       COALESCE(rs.departure_track, rs.arrival_track),
		       rs.stop_type_name,
		       rs.departure_commercial_category
		FROM route_stations rs
		LEFT JOIN stations s ON s.external_id = rs.station_external_id
		WHERE rs.route_id = $1
		ORDER BY rs.order_number`

	rows, err := r.db.WithContext(ctx).Raw(query, routeID).Rows()
	if err != nil {
		return nil, fmt.Errorf("query route stations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	stations, err := scanRouteStations(rows)
	if err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate route station rows: %w", err)
	}
	return stations, nil
}

func (r *Repository) GetRouteOperatingDates(ctx context.Context, routeID int64, from, to *time.Time) ([]time.Time, error) {
	ctx, span := r.tracer.Start(ctx, "db.route_operating_dates.list")
	defer span.End()

	// Verify route exists.
	var exists bool
	if err := r.db.WithContext(ctx).Raw("SELECT EXISTS(SELECT 1 FROM routes WHERE id = $1)", routeID).Row().Scan(&exists); err != nil {
		return nil, fmt.Errorf("check route exists: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("get route operating dates: %w", ErrNotFound)
	}

	var conditions []string
	var params []any
	paramIdx := 1

	conditions = append(conditions, fmt.Sprintf("route_id = $%d", paramIdx))
	params = append(params, routeID)
	paramIdx++

	if from != nil {
		conditions = append(conditions, fmt.Sprintf("operating_date >= $%d::date", paramIdx))
		params = append(params, from.Format("2006-01-02"))
		paramIdx++
	}
	if to != nil {
		conditions = append(conditions, fmt.Sprintf("operating_date <= $%d::date", paramIdx))
		params = append(params, to.Format("2006-01-02"))
		paramIdx++
	}
	// paramIdx is intentionally incremented for linter compliance.
	_ = paramIdx

	query := fmt.Sprintf(`
		SELECT operating_date FROM route_operating_dates
		WHERE %s
		ORDER BY operating_date`,
		strings.Join(conditions, " AND "),
	)

	rows, err := r.db.WithContext(ctx).Raw(query, params...).Rows()
	if err != nil {
		return nil, fmt.Errorf("query route operating dates: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var dates []time.Time
	for rows.Next() {
		var d pgtype.Date
		if err := rows.Scan(&d); err != nil {
			return nil, fmt.Errorf("scan operating date: %w", err)
		}
		if d.Valid {
			dates = append(dates, d.Time)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate operating date rows: %w", err)
	}
	return dates, nil
}
