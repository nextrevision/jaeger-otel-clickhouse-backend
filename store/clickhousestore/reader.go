package clickhousestore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
	"log/slog"
	"regexp"
	"strings"
	"time"
)

var (
	ErrNotFound = errors.New("not found")
)

type ClickhouseStore interface {
	GetServices(ctx context.Context) ([]string, error)
	GetSpanNames(ctx context.Context, serviceName string) ([]string, error)
	GetTrace(ctx context.Context, traceID string) (*ClickhouseOtelTrace, error)
	GetTraces(ctx context.Context, traceIDs []string) ([]*ClickhouseOtelTrace, error)
	SearchTraces(ctx context.Context, serviceName string, startTime time.Time, endTime time.Time, options SearchOptions) ([]string, error)
}

type ClickhouseReader struct {
	table      string
	padTraceID bool
	db         *sql.DB
	tracer     trace.Tracer
	logger     *slog.Logger
}

func New(table string, padTraceID bool, db *sql.DB, tracer trace.Tracer) *ClickhouseReader {
	return &ClickhouseReader{
		table:      table,
		padTraceID: padTraceID,
		db:         db,
		tracer:     tracer,
		logger:     slog.Default(),
	}
}

func (r *ClickhouseReader) GetServices(ctx context.Context) ([]string, error) {
	ctx, span := r.tracer.Start(ctx, "clickhousereader:GetServices")
	defer span.End()

	query := fmt.Sprintf("SELECT DISTINCT ServiceName FROM %s GROUP BY ServiceName", r.table)

	return r.queryToStrings(ctx, query)
}

func (r *ClickhouseReader) GetSpanNames(ctx context.Context, serviceName string) ([]string, error) {
	ctx, span := r.tracer.Start(ctx, "clickhousereader:GetSpanNames")
	defer span.End()

	query := fmt.Sprintf("SELECT DISTINCT SpanName FROM %s WHERE ServiceName = ? GROUP BY SpanName", r.table)
	args := []interface{}{serviceName}

	return r.queryToStrings(ctx, query, args...)
}

func (r *ClickhouseReader) GetTrace(ctx context.Context, traceID string) (*ClickhouseOtelTrace, error) {
	ctx, span := r.tracer.Start(ctx, "clickhousereader:GetTrace")
	span.SetAttributes(attribute.String("trace-id", traceID))
	defer span.End()

	traces, err := r.getTraces(ctx, []string{traceID})
	if err != nil {
		return &ClickhouseOtelTrace{}, err
	}

	if len(traces) == 0 {
		return &ClickhouseOtelTrace{}, ErrNotFound
	}

	return traces[0], nil
}

func (r *ClickhouseReader) GetTraces(ctx context.Context, traceIDs []string) ([]*ClickhouseOtelTrace, error) {
	ctx, span := r.tracer.Start(ctx, "clickhousereader:GetTrace")
	span.SetAttributes(attribute.StringSlice("trace-ids", traceIDs))
	defer span.End()

	return r.getTraces(ctx, traceIDs)
}

func (r *ClickhouseReader) SearchTraces(ctx context.Context, serviceName string, startTime time.Time, endTime time.Time, options SearchOptions) ([]string, error) {
	ctx, span := r.tracer.Start(ctx, "clickhousereader:SearchTraces")
	span.SetAttributes(attribute.String("service-name", serviceName))
	defer span.End()

	if endTime.Before(startTime) || endTime.UTC() == startTime.UTC() {
		return []string{}, nil
	}

	span.SetAttributes(attribute.String("time-range", endTime.Sub(startTime).String()))

	args := []interface{}{}
	query := fmt.Sprintf("SELECT DISTINCT TraceId FROM %s WHERE ServiceName = ?", r.table)
	args = append(args, serviceName)

	if options.SpanName != "" {
		query = query + " AND SpanName = ?"
		args = append(args, options.SpanName)
	}

	query = query + " AND (Timestamp >= toDateTime(?) AND Timestamp <= toDateTime(?))"
	args = append(args, startTime.Unix(), endTime.Unix())

	if options.MinDuration != 0 {
		query = query + " AND Duration >= ?"
		args = append(args, options.MinDuration.Nanoseconds())
	}

	if options.MaxDuration != 0 {
		query = query + " AND Duration <= ?"
		args = append(args, options.MaxDuration.Nanoseconds())
	}

	for key, value := range options.Attributes {
		// Check for instances of wildcard without being escaped
		wildcardMatch, _ := regexp.MatchString(`(^|[^\\])%`, value)
		if strings.HasPrefix(value, "~") {
			value = strings.TrimLeft(value, "~")
			span.SetAttributes(attribute.String("query-type", "MATCH"))
			span.SetAttributes(attribute.String("query-key", key))
			span.SetAttributes(attribute.String("query-value", value))
			query = query + " AND match(SpanAttributes[?], ?)"
			args = append(args, key, value)
		} else if wildcardMatch {
			span.SetAttributes(attribute.String("query-type", "LIKE"))
			span.SetAttributes(attribute.String("query-key", key))
			span.SetAttributes(attribute.String("query-value", value))
			query = query + " AND (SpanAttributes[?] LIKE ? AND SpanAttributes[?] != '')"
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
			query = query + " AND SpanAttributes[?] = ?"
			args = append(args, key, value)
		}
	}

	if len(options.IgnoredTraceIDs) > 0 {
		query = query + fmt.Sprintf(" AND TraceId NOT IN (%s)", "?"+strings.Repeat(",?", len(options.IgnoredTraceIDs)-1))
		for _, traceID := range options.IgnoredTraceIDs {
			args = append(args, traceID)
		}
	}

	// Sorting by service is required for early termination of primary key scan:
	// * https://github.com/ClickHouse/ClickHouse/issues/7102
	query = query + " ORDER BY ServiceName, -toUnixTimestamp(Timestamp) LIMIT ?"
	args = append(args, options.SearchLimit-len(options.IgnoredTraceIDs))

	return r.queryToStrings(ctx, query, args...)
}

func (r *ClickhouseReader) queryToStrings(ctx context.Context, sql string, args ...interface{}) ([]string, error) {
	ctx, span := r.tracer.Start(ctx, "clickhousereader:queryToStrings")
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

func (r *ClickhouseReader) getTraces(ctx context.Context, traceIDs []string) ([]*ClickhouseOtelTrace, error) {
	ctx, span := r.tracer.Start(ctx, "clickhousereader:getTraces")
	span.SetAttributes(attribute.StringSlice("trace-ids", traceIDs))
	defer span.End()

	var traces []*ClickhouseOtelTrace

	if len(traceIDs) == 0 {
		return traces, nil
	}

	// Normalize trace IDs to contain 32 characters with zeros prepended
	if r.padTraceID {
		traceIDs = r.padTraceIDs(traceIDs)
	}

	traceIDSearch := make([]interface{}, len(traceIDs))
	for i, traceID := range traceIDs {
		traceIDSearch[i] = traceID
	}

	query := fmt.Sprintf(
		"SELECT Timestamp, TraceId, SpanId, ParentSpanId, TraceState, SpanName, SpanKind, ServiceName, ResourceAttributes, ScopeName, ScopeVersion, SpanAttributes, Duration, StatusCode, StatusMessage, Events.Timestamp, Events.Name, Events.Attributes FROM %s PREWHERE TraceId IN (%s)",
		r.table,
		"?"+strings.Repeat(",?", len(traceIDSearch)-1),
	)

	span.SetAttributes(
		semconv.DBSystemClickhouse,
		semconv.DBStatement(query),
		semconv.DBSQLTable(r.table),
	)

	rows, err := r.db.QueryContext(ctx, query, traceIDSearch...)
	if err != nil {
		r.logger.ErrorContext(ctx, "unable to execute query", "error", err)
		span.SetStatus(codes.Error, "unable to execute query")
		span.RecordError(err)
		return nil, err
	}

	defer func() { _ = rows.Close() }()

	traceMap := map[string]*ClickhouseOtelTrace{}

	for rows.Next() {
		var s ClickhouseOtelSpan

		if err := rows.Scan(
			&s.Timestamp, &s.TraceID, &s.SpanID, &s.ParentSpanID, &s.TraceState, &s.SpanName, &s.SpanKind,
			&s.ServiceName, &s.ResourceAttributes, &s.ScopeName, &s.ScopeVersion, &s.SpanAttributes, &s.Duration,
			&s.StatusCode, &s.StatusMessage, &s.EventsTimestamp, &s.EventsName, &s.EventsAttributes,
		); err != nil {
			r.logger.ErrorContext(ctx, "unable to map to structure", "error", err)
			span.SetStatus(codes.Error, "unable to map to structure")
			span.RecordError(err)
			return nil, err
		}

		if _, ok := traceMap[s.TraceID]; !ok {
			traceMap[s.TraceID] = &ClickhouseOtelTrace{TraceID: s.TraceID}
		}
		traceMap[s.TraceID].Spans = append(traceMap[s.TraceID].Spans, s)
	}

	for _, t := range traceMap {
		traces = append(traces, t)
	}

	return traces, nil
}

// Normalize trace IDs to contain 32 characters with zeros prepended
func (r *ClickhouseReader) padTraceIDs(traceIDs []string) []string {
	paddedTraceIDs := make([]string, 0, len(traceIDs))
	for _, traceID := range traceIDs {
		if len(traceID) == 16 {
			paddedTraceIDs = append(paddedTraceIDs, fmt.Sprintf("%032s", traceID))
		} else {
			paddedTraceIDs = append(paddedTraceIDs, traceID)
		}
	}
	return paddedTraceIDs
}
