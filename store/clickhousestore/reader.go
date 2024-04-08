package clickhousestore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
	"log/slog"
	"regexp"
	"strings"
	"time"
)

const (
	minTimespanForProgressiveSearch       = time.Hour
	minTimespanForProgressiveSearchMargin = time.Minute
	maxProgressiveSteps                   = 4
)

var (
	ErrStartTimeRequired = errors.New("start time is required for search queries")
)

type ClickhouseReader struct {
	db     *sql.DB
	table  string
	tracer trace.Tracer
	logger *slog.Logger
}

func New(table string, db *sql.DB, tracer trace.Tracer) *ClickhouseReader {
	return &ClickhouseReader{
		db:     db,
		table:  table,
		tracer: tracer,
		logger: slog.Default(),
	}
}

func (r *ClickhouseReader) GetTrace(ctx context.Context, traceID model.TraceID) (*model.Trace, error) {
	ctx, span := r.tracer.Start(ctx, "grpc:GetTrace")
	defer span.End()

	traces, err := r.getTraces(ctx, []model.TraceID{traceID})
	if err != nil {
		return nil, err
	}

	if len(traces) == 0 {
		r.logger.WarnContext(ctx, "no trace found", "traceId", traceID.String())
		return nil, spanstore.ErrTraceNotFound
	}

	return traces[0], nil
}

func (r *ClickhouseReader) GetServices(ctx context.Context) ([]string, error) {
	ctx, span := r.tracer.Start(ctx, "grpc:GetServices")
	defer span.End()

	query := fmt.Sprintf("SELECT DISTINCT ServiceName FROM %s GROUP BY ServiceName", r.table)

	return r.getStrings(ctx, query)
}

func (r *ClickhouseReader) GetOperations(ctx context.Context, query spanstore.OperationQueryParameters) ([]spanstore.Operation, error) {
	ctx, span := r.tracer.Start(ctx, "grpc:GetOperations")
	defer span.End()

	stmt := fmt.Sprintf("SELECT DISTINCT SpanName FROM %s WHERE ServiceName = ? GROUP BY SpanName", r.table)
	args := []interface{}{query.ServiceName}

	names, err := r.getStrings(ctx, stmt, args...)
	if err != nil {
		return nil, err
	}

	operations := make([]spanstore.Operation, len(names))
	for i, name := range names {
		operations[i].Name = name
	}

	return operations, nil
}

func (r *ClickhouseReader) FindTraces(ctx context.Context, query *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	ctx, span := r.tracer.Start(ctx, "grpc:FindTraces")
	defer span.End()

	traceIDs, err := r.FindTraceIDs(ctx, query)
	if err != nil {
		return nil, err
	}

	return r.getTraces(ctx, traceIDs)
}

func (r *ClickhouseReader) FindTraceIDs(ctx context.Context, query *spanstore.TraceQueryParameters) ([]model.TraceID, error) {
	ctx, span := r.tracer.Start(ctx, "grpc:FindTraceIDs")
	defer span.End()

	if query.StartTimeMin.IsZero() {
		return nil, ErrStartTimeRequired
	}

	end := query.StartTimeMax
	if end.IsZero() {
		end = time.Now()
	}

	fullTimeSpan := end.Sub(query.StartTimeMin)

	if fullTimeSpan < minTimespanForProgressiveSearch+minTimespanForProgressiveSearchMargin {
		return r.findTraceIDsInRange(ctx, query, query.StartTimeMin, end, nil)
	}

	timeSpan := fullTimeSpan
	for step := 0; step < maxProgressiveSteps; step++ {
		timeSpan /= 2
	}

	if timeSpan < minTimespanForProgressiveSearch {
		timeSpan = minTimespanForProgressiveSearch
	}

	found := []model.TraceID{}

	for step := 0; step < maxProgressiveSteps; step++ {
		if len(found) >= query.NumTraces {
			break
		}

		// last step has to take care of the whole remainder
		if step == maxProgressiveSteps-1 {
			timeSpan = fullTimeSpan
		}

		start := end.Add(-timeSpan)
		if start.Before(query.StartTimeMin) {
			start = query.StartTimeMin
		}

		if start.After(end) {
			break
		}

		foundInRange, err := r.findTraceIDsInRange(ctx, query, start, end, found)
		if err != nil {
			return nil, err
		}

		found = append(found, foundInRange...)

		end = start
		timeSpan = timeSpan * 2
	}

	return found, nil
}

func (r *ClickhouseReader) GetDependencies(ctx context.Context, endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error) {
	ctx, span := r.tracer.Start(ctx, "grpc:GetDependencies")
	defer span.End()

	return nil, nil
}

func (r *ClickhouseReader) getStrings(ctx context.Context, sql string, args ...interface{}) ([]string, error) {
	ctx, span := r.tracer.Start(ctx, "reader:getStrings")
	defer span.End()
	span.SetAttributes(
		semconv.DBSystemClickhouse,
		semconv.DBStatement(sql),
		semconv.DBSQLTable(r.table),
	)

	rows, err := r.db.QueryContext(ctx, sql, args...)
	if err != nil {
		r.logger.ErrorContext(ctx, "unable to execute query", "error", err)
		span.SetStatus(codes.Error, "unable to execute query")
		span.RecordError(err)
		return nil, err
	}

	defer func() { _ = rows.Close() }()

	values := []string{}

	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			r.logger.ErrorContext(ctx, "unable to scan row results", "error", err)
			span.SetStatus(codes.Error, "unable to scan row results")
			span.RecordError(err)
			return nil, err
		}
		values = append(values, value)
	}

	if err := rows.Err(); err != nil {
		r.logger.ErrorContext(ctx, "received errors in rows", "error", err)
		span.SetStatus(codes.Error, "received errors in rows")
		span.RecordError(err)
		return nil, err
	}

	return values, nil
}

func (r *ClickhouseReader) getTraces(ctx context.Context, traceIDs []model.TraceID) ([]*model.Trace, error) {
	ctx, span := r.tracer.Start(ctx, "reader:getTraces")
	defer span.End()

	var returning []*model.Trace

	if len(traceIDs) == 0 {
		return returning, nil
	}

	traceIDSearch := make([]interface{}, len(traceIDs))
	traceIDStrings := make([]string, len(traceIDs))
	for i, traceID := range traceIDs {
		traceIDSearch[i] = traceID.String()
		traceIDStrings[i] = traceID.String()
	}
	span.SetAttributes(attribute.StringSlice("trace-ids", traceIDStrings))

	// It's more efficient to do PREWHERE on traceID to then only read needed models:
	// * https://clickhouse.tech/docs/en/sql-reference/statements/select/prewhere/
	stmt := fmt.Sprintf(
		"SELECT Timestamp, TraceId, SpanId, ParentSpanId, TraceState, SpanName, SpanKind, ServiceName, ResourceAttributes, ScopeName, ScopeVersion, SpanAttributes, Duration, StatusCode, StatusMessage, Events.Timestamp, Events.Name, Events.Attributes FROM %s PREWHERE TraceId IN (%s)",
		r.table,
		"?"+strings.Repeat(",?", len(traceIDSearch)-1),
	)

	span.SetAttributes(
		semconv.DBSystemClickhouse,
		semconv.DBStatement(stmt),
		semconv.DBSQLTable(r.table),
	)
	rows, err := r.db.QueryContext(ctx, stmt, traceIDSearch...)
	if err != nil {
		r.logger.ErrorContext(ctx, "unable to execute query", "error", err)
		span.SetStatus(codes.Error, "unable to execute query")
		span.RecordError(err)
		return nil, err
	}

	defer func() { _ = rows.Close() }()

	traces := map[model.TraceID]*model.Trace{}

	for rows.Next() {
		var coSpan ClickhouseOtelSpan

		if err := rows.Scan(&coSpan.Timestamp, &coSpan.TraceId, &coSpan.SpanId, &coSpan.ParentSpanId, &coSpan.TraceState, &coSpan.SpanName, &coSpan.SpanKind, &coSpan.ServiceName, &coSpan.ResourceAttributes, &coSpan.ScopeName, &coSpan.ScopeVersion, &coSpan.SpanAttributes, &coSpan.Duration, &coSpan.StatusCode, &coSpan.StatusMessage, &coSpan.EventsTimestamp, &coSpan.EventsName, &coSpan.EventsAttributes); err != nil {
			r.logger.ErrorContext(ctx, "unable to map to structure", "error", err)
			span.SetStatus(codes.Error, "unable to map to structure")
			span.RecordError(err)
			return nil, err
		}

		traceID, err := model.TraceIDFromString(coSpan.TraceId)
		if err != nil {
			r.logger.ErrorContext(ctx, "unable to normalize trace id", "error", err)
			span.SetStatus(codes.Error, fmt.Sprintf("unable to normalize trace id %s", coSpan.TraceId))
			span.RecordError(err)
			return nil, err
		}

		spanID, err := model.SpanIDFromString(coSpan.SpanId)
		if err != nil {
			r.logger.ErrorContext(ctx, "unable to normalize span id", "error", err)
			span.SetStatus(codes.Error, fmt.Sprintf("unable to normalize span id %s", coSpan.SpanId))
			span.RecordError(err)
			return nil, err
		}

		newSpan := model.Span{
			TraceID:       traceID,
			SpanID:        spanID,
			OperationName: coSpan.SpanName,
		}

		newSpan.StartTime = coSpan.Timestamp

		newSpan.Duration = time.Duration(coSpan.Duration)

		if coSpan.ParentSpanId != "" {
			parentSpanID, err := model.SpanIDFromString(coSpan.ParentSpanId)
			if err != nil {
				r.logger.ErrorContext(ctx, "unable to normalize parent span id", "error", err)
				span.SetStatus(codes.Error, fmt.Sprintf("unable to normalize parent span id %s", coSpan.ParentSpanId))
				span.RecordError(err)
				return nil, err
			}
			newSpan.References = []model.SpanRef{
				{
					TraceID: traceID,
					SpanID:  parentSpanID,
					RefType: model.SpanRefType_CHILD_OF,
				},
			}
		}

		if len(coSpan.SpanAttributes) > 0 {
			tags := make([]model.KeyValue, 0, len(coSpan.SpanAttributes))
			for key, value := range coSpan.SpanAttributes {
				tags = append(tags, model.String(key, value))
			}
			newSpan.Tags = tags
		}

		if len(coSpan.EventsName) > 0 {
			logs := make([]model.Log, 0, len(coSpan.EventsName))
			for idx, value := range coSpan.EventsName {
				log := model.Log{}
				log.Timestamp = coSpan.EventsTimestamp[idx]
				log.Fields = make([]model.KeyValue, len(coSpan.EventsAttributes)+1)
				log.Fields = append(log.Fields, model.String("event", value))
				for i := range coSpan.EventsAttributes {
					for k, v := range coSpan.EventsAttributes[i] {
						log.Fields = append(log.Fields, model.String(k, v))
					}
				}
				logs = append(logs, log)
			}
			newSpan.Logs = logs
		}

		if len(coSpan.ResourceAttributes) > 0 {
			process := &model.Process{ServiceName: coSpan.ServiceName}
			process.Tags = make([]model.KeyValue, 0, len(coSpan.ResourceAttributes))
			for key, value := range coSpan.ResourceAttributes {
				process.Tags = append(process.Tags, model.String(key, value))
			}
			newSpan.Process = process
		}

		if _, ok := traces[newSpan.TraceID]; !ok {
			traces[newSpan.TraceID] = &model.Trace{}
		}

		traces[newSpan.TraceID].Spans = append(traces[newSpan.TraceID].Spans, &newSpan)
	}

	if err := rows.Err(); err != nil {
		r.logger.ErrorContext(ctx, "rows returned errors", "error", err)
		span.SetStatus(codes.Error, "rows returned errors")
		span.RecordError(err)
		return nil, err
	}

	for _, traceID := range traceIDs {
		if trace, ok := traces[traceID]; ok {
			returning = append(returning, trace)
		}
	}

	return returning, nil
}

func (r *ClickhouseReader) findTraceIDsInRange(ctx context.Context, query *spanstore.TraceQueryParameters, start, end time.Time, skip []model.TraceID) ([]model.TraceID, error) {
	ctx, span := r.tracer.Start(ctx, "reader:findTraceIDsInRange")
	defer span.End()

	if end.Before(start) || end.UTC() == start.UTC() {
		return []model.TraceID{}, nil
	}

	span.SetAttributes(attribute.String("range", end.Sub(start).String()))

	args := []interface{}{}
	stmt := fmt.Sprintf("SELECT DISTINCT TraceId FROM %s WHERE ServiceName = ?", r.table)
	args = append(args, query.ServiceName)

	if query.OperationName != "" {
		stmt = stmt + " AND SpanName = ?"
		args = append(args, query.OperationName)
	}

	stmt = stmt + " AND (Timestamp >= toDateTime(?) AND Timestamp <= toDateTime(?))"
	args = append(args, start.Unix(), end.Unix())

	if query.DurationMin != 0 {
		stmt = stmt + " AND Duration >= ?"
		args = append(args, query.DurationMin.Nanoseconds())
	}

	if query.DurationMax != 0 {
		stmt = stmt + " AND Duration <= ?"
		args = append(args, query.DurationMax.Nanoseconds())
	}

	for key, value := range query.Tags {
		// Check for instances of wildcard without being escaped
		wildcardMatch, _ := regexp.MatchString(`(^|[^\\])%`, value)
		if strings.HasPrefix(value, "~") {
			value = strings.TrimLeft(value, "~")
			span.SetAttributes(attribute.String("query-type", "MATCH"))
			span.SetAttributes(attribute.String("query-key", key))
			span.SetAttributes(attribute.String("query-value", value))
			stmt = stmt + fmt.Sprintf(" AND match(SpanAttributes[?], '%s')", value)
			args = append(args, key)
		} else if wildcardMatch {
			span.SetAttributes(attribute.String("query-type", "LIKE"))
			span.SetAttributes(attribute.String("query-key", key))
			span.SetAttributes(attribute.String("query-value", value))
			stmt = stmt + " AND (SpanAttributes[?] LIKE ? AND SpanAttributes[?] != '')"
			args = append(args, key, value, key)
		} else {
			// Replace all escaped wildcards with literal '%'
			// This is a janky workaround to support wildcard matches while also supporting
			// literal '%' queries. If the query only contains literals by way of escaping,
			// normalize them into the proper query syntax.
			value = strings.ReplaceAll(value, "\\%", "%")
			span.SetAttributes(attribute.String("query-type", "EQUAL"))
			span.SetAttributes(attribute.String("query-key", key))
			span.SetAttributes(attribute.String("query-value", value))
			stmt = stmt + " AND SpanAttributes[?] = ?"
			args = append(args, key, value)
		}
	}

	if len(skip) > 0 {
		stmt = stmt + fmt.Sprintf(" AND TraceId NOT IN (%s)", "?"+strings.Repeat(",?", len(skip)-1))
		for _, traceID := range skip {
			args = append(args, traceID.String())
		}
	}

	// Sorting by service is required for early termination of primary key scan:
	// * https://github.com/ClickHouse/ClickHouse/issues/7102
	stmt = stmt + " ORDER BY ServiceName, -toUnixTimestamp(Timestamp) LIMIT ?"
	args = append(args, query.NumTraces-len(skip))

	traceIDStrings, err := r.getStrings(ctx, stmt, args...)
	if err != nil {
		return nil, err
	}

	traceIDs := make([]model.TraceID, len(traceIDStrings))
	for i, traceIDString := range traceIDStrings {
		traceID, err := model.TraceIDFromString(traceIDString)
		if err != nil {
			r.logger.ErrorContext(ctx, "unable to get trace id from string", "error", err)
			span.SetStatus(codes.Error, fmt.Sprintf("unable to get trace id from string: %s", traceIDString))
			span.RecordError(err)
			return nil, err
		}
		traceIDs[i] = traceID
	}

	return traceIDs, nil
}
