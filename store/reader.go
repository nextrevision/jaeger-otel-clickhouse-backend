package store

import (
	"context"
	"errors"
	"fmt"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	"github.com/nextrevision/jaeger-otel-clickhouse-backend/store/clickhousestore"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
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

func (s *Store) GetTrace(ctx context.Context, traceID model.TraceID) (*model.Trace, error) {
	ctx, span := s.tracer.Start(ctx, "grpc:GetTrace")
	defer span.End()

	trace, err := s.clickhousestore.GetTrace(ctx, traceID.String())
	if errors.Is(err, clickhousestore.ErrNotFound) {
		s.logger.WarnContext(ctx, "no trace found", "traceId", traceID.String())
		return nil, spanstore.ErrTraceNotFound
	} else if err != nil {
		return nil, err
	}

	return s.convertClickhouseToJaegerTrace(ctx, trace)
}

func (s *Store) GetServices(ctx context.Context) ([]string, error) {
	ctx, span := s.tracer.Start(ctx, "grpc:GetServices")
	defer span.End()

	return s.clickhousestore.GetServices(ctx)
}

func (s *Store) GetOperations(ctx context.Context, query spanstore.OperationQueryParameters) ([]spanstore.Operation, error) {
	ctx, span := s.tracer.Start(ctx, "grpc:GetOperations")
	defer span.End()

	names, err := s.clickhousestore.GetSpanNames(ctx, query.ServiceName)
	if err != nil {
		return nil, err
	}

	operations := make([]spanstore.Operation, 0, len(names))
	for _, name := range names {
		operations = append(operations, spanstore.Operation{Name: name})
	}

	return operations, nil
}

func (s *Store) FindTraces(ctx context.Context, query *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	ctx, span := s.tracer.Start(ctx, "grpc:FindTraces")
	defer span.End()

	traceIDs, err := s.FindTraceIDs(ctx, query)
	if err != nil {
		return nil, err
	}

	traceIDStrings := make([]string, 0, len(traceIDs))
	for _, traceID := range traceIDs {
		traceIDStrings = append(traceIDStrings, traceID.String())
	}

	traces, err := s.clickhousestore.GetTraces(ctx, traceIDStrings)
	if err != nil {
		s.logger.ErrorContext(ctx, "unable to get traces from clickhousestore", "error", err)
		span.SetStatus(codes.Error, "unable to get traces from clickhousestore")
		span.RecordError(err)
	}

	jaegerTraces := make([]*model.Trace, 0, len(traces))
	for _, t := range traces {
		jaegerTrace, err := s.convertClickhouseToJaegerTrace(ctx, t)
		if err != nil {
			return nil, err
		}
		jaegerTraces = append(jaegerTraces, jaegerTrace)
	}

	return jaegerTraces, nil
}

func (s *Store) FindTraceIDs(ctx context.Context, query *spanstore.TraceQueryParameters) ([]model.TraceID, error) {
	ctx, span := s.tracer.Start(ctx, "grpc:FindTraceIDs")
	defer span.End()

	searchOptions := clickhousestore.SearchOptions{
		SpanName:    query.OperationName,
		Attributes:  query.Tags,
		MinDuration: query.DurationMin,
		MaxDuration: query.DurationMax,
		SearchLimit: query.NumTraces,
	}

	if query.StartTimeMin.IsZero() {
		return nil, ErrStartTimeRequired
	}

	end := query.StartTimeMax
	if end.IsZero() {
		end = time.Now()
	}

	fullTimeSpan := end.Sub(query.StartTimeMin)

	if fullTimeSpan < minTimespanForProgressiveSearch+minTimespanForProgressiveSearchMargin {
		traces, err := s.clickhousestore.SearchTraces(ctx, query.ServiceName, query.StartTimeMin, end, searchOptions)
		if err != nil {
			return nil, err
		}

		jaegerTraces := []model.TraceID{}
		for _, traceID := range traces {
			jaegerTraceID, err := s.traceStringToID(ctx, traceID)
			if err != nil {
				return nil, err
			}
			jaegerTraces = append(jaegerTraces, jaegerTraceID)
		}
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

		// Add trace IDs
		for _, trace := range found {
			searchOptions.IgnoredTraceIDs = append(searchOptions.IgnoredTraceIDs, trace.String())
		}

		foundInRange, err := s.clickhousestore.SearchTraces(ctx, query.ServiceName, start, end, searchOptions)
		if err != nil {
			return nil, err
		}

		for _, traceID := range foundInRange {
			jaegerTraceID, err := s.traceStringToID(ctx, traceID)
			if err != nil {
				return nil, err
			}
			found = append(found, jaegerTraceID)
		}

		end = start
		timeSpan = timeSpan * 2
	}

	return found, nil
}

func (s *Store) GetDependencies(ctx context.Context, endTime time.Time, lookback time.Duration) ([]model.DependencyLink, error) {
	ctx, span := s.tracer.Start(ctx, "grpc:GetDependencies")
	defer span.End()

	return nil, nil
}

func (s *Store) traceStringToID(ctx context.Context, traceIDString string) (model.TraceID, error) {
	ctx, span := s.tracer.Start(ctx, "store:traceStringToID")
	span.SetAttributes(attribute.String("trace-id", traceIDString))
	defer span.End()

	traceID, err := model.TraceIDFromString(traceIDString)
	if err != nil {
		s.logger.ErrorContext(ctx, "unable to normalize trace id", "error", err)
		span.SetStatus(codes.Error, fmt.Sprintf("unable to normalize trace id %s", traceID))
		span.RecordError(err)
		return traceID, err
	}

	return traceID, nil
}

func (s *Store) convertClickhouseToJaegerTrace(ctx context.Context, chTrace *clickhousestore.ClickhouseOtelTrace) (*model.Trace, error) {
	ctx, span := s.tracer.Start(ctx, "store:convertClickhouseToJaegerTrace")
	span.SetAttributes(attribute.String("trace-id", chTrace.TraceID))
	defer span.End()

	jaegerTrace := &model.Trace{}

	traceID, err := model.TraceIDFromString(chTrace.TraceID)
	if err != nil {
		s.logger.ErrorContext(ctx, "unable to normalize trace id", "error", err)
		span.SetStatus(codes.Error, fmt.Sprintf("unable to normalize trace id %s", chTrace.TraceID))
		span.RecordError(err)
		return nil, err
	}

	for _, sp := range chTrace.Spans {

		spanID, err := model.SpanIDFromString(sp.SpanID)
		if err != nil {
			s.logger.ErrorContext(ctx, "unable to normalize span id", "error", err)
			span.SetStatus(codes.Error, fmt.Sprintf("unable to normalize span id %s", sp.SpanID))
			span.RecordError(err)
			return nil, err
		}

		newSpan := &model.Span{
			TraceID:       traceID,
			SpanID:        spanID,
			OperationName: sp.SpanName,
			StartTime:     sp.Timestamp,
			Duration:      time.Duration(sp.Duration),
		}

		if sp.ParentSpanID != "" {
			parentSpanID, err := model.SpanIDFromString(sp.ParentSpanID)
			if err != nil {
				s.logger.ErrorContext(ctx, "unable to normalize parent span id", "error", err)
				span.SetStatus(codes.Error, fmt.Sprintf("unable to normalize parent span id %s", sp.ParentSpanID))
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

		if len(sp.SpanAttributes) > 0 {
			tags := make([]model.KeyValue, 0, len(sp.SpanAttributes))
			for key, value := range sp.SpanAttributes {
				tags = append(tags, model.String(key, value))
			}
			if sp.StatusCode == "STATUS_CODE_ERROR" {
				tags = append(tags, model.Bool("error", true))
			}
			newSpan.Tags = tags
		}

		if len(sp.EventsName) > 0 {
			logs := make([]model.Log, 0, len(sp.EventsName))
			for idx, value := range sp.EventsName {
				log := model.Log{}
				log.Timestamp = sp.EventsTimestamp[idx]
				log.Fields = make([]model.KeyValue, 0, len(sp.EventsAttributes)+1)
				log.Fields = append(log.Fields, model.String("event", value))
				for i := range sp.EventsAttributes {
					for k, v := range sp.EventsAttributes[i] {
						log.Fields = append(log.Fields, model.String(k, v))
					}
				}
				logs = append(logs, log)
			}
			newSpan.Logs = logs
		}

		if len(sp.ResourceAttributes) > 0 {
			process := &model.Process{ServiceName: sp.ServiceName}
			process.Tags = make([]model.KeyValue, 0, len(sp.ResourceAttributes))
			for key, value := range sp.ResourceAttributes {
				process.Tags = append(process.Tags, model.String(key, value))
			}
			newSpan.Process = process
		}

		jaegerTrace.Spans = append(jaegerTrace.Spans, newSpan)
	}

	return jaegerTrace, nil
}
