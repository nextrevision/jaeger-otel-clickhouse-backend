package store

import (
	"context"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	"github.com/nextrevision/jaeger-otel-clickhouse-backend/store/clickhousestore"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/trace/noop"
	"testing"
	"time"
)

func TestStore_GetTrace(t *testing.T) {
	mockReader := clickhousestore.NewMockClickhouseReader(2)
	tracer := noop.Tracer{}

	store := New(mockReader, tracer)
	ctx := context.Background()

	traceIDOne, _ := model.TraceIDFromString(clickhousestore.TestDataTraceIDOne)

	got, err := store.GetTrace(ctx, traceIDOne)
	if err != nil {
		t.Errorf("Store.GetTrace() error = %v", err)
		return
	}

	assert.Equal(t, 2, len(got.Spans))
	assert.Equal(t, traceIDOne, got.Spans[0].TraceID)
	assert.Equal(t, clickhousestore.TestDataSpanNameOne, got.Spans[0].OperationName)
	assert.Equal(t, time.Duration(3600), got.Spans[0].Duration)
	assert.Equal(t, 2, len(got.Spans[0].Tags))
	assert.Contains(t, got.Spans[0].Tags[0].Key, "attr")
	assert.Contains(t, got.Spans[0].Tags[0].Value(), "value")
	assert.Contains(t, got.Spans[0].Tags[1].Key, "attr")
	assert.Contains(t, got.Spans[0].Tags[1].Value(), "value")
	assert.Equal(t, traceIDOne, got.Spans[1].TraceID)
	assert.Equal(t, clickhousestore.TestDataSpanNameTwo, got.Spans[1].OperationName)
	assert.Equal(t, time.Duration(3600), got.Spans[1].Duration)
	assert.Equal(t, 0, len(got.Spans[1].Tags))
}

func TestStore_GetServices(t *testing.T) {
	mockReader := clickhousestore.NewMockClickhouseReader(2)
	tracer := noop.Tracer{}

	store := New(mockReader, tracer)
	ctx := context.Background()

	got, err := store.GetServices(ctx)
	if err != nil {
		t.Errorf("Store.GetTrace() error = %v", err)
		return
	}

	assert.Equal(t, 2, len(got))
	assert.Equal(t, clickhousestore.TestDataServiceNameOne, got[0])
	assert.Equal(t, clickhousestore.TestDataServiceNameTwo, got[1])
}

func TestStore_GetOperations(t *testing.T) {
	mockReader := clickhousestore.NewMockClickhouseReader(2)
	tracer := noop.Tracer{}

	store := New(mockReader, tracer)
	ctx := context.Background()

	query := spanstore.OperationQueryParameters{
		ServiceName: clickhousestore.TestDataServiceNameOne,
	}

	got, err := store.GetOperations(ctx, query)
	if err != nil {
		t.Errorf("Store.GetTrace() error = %v", err)
		return
	}

	assert.Equal(t, 2, len(got))

	assert.Equal(t, spanstore.Operation{Name: clickhousestore.TestDataSpanNameOne}, got[0])
	assert.Equal(t, spanstore.Operation{Name: clickhousestore.TestDataSpanNameTwo}, got[1])
}

func TestStore_GetFindTraces(t *testing.T) {
	mockReader := clickhousestore.NewMockClickhouseReader(1)
	tracer := noop.Tracer{}

	store := New(mockReader, tracer)
	ctx := context.Background()

	query := &spanstore.TraceQueryParameters{
		ServiceName:  clickhousestore.TestDataServiceNameOne,
		StartTimeMin: time.Now().AddDate(0, 0, -1),
		StartTimeMax: time.Now(),
	}

	got, err := store.FindTraces(ctx, query)
	if err != nil {
		t.Errorf("Store.GetTrace() error = %v", err)
		return
	}

	traceIDOne, _ := model.TraceIDFromString(clickhousestore.TestDataTraceIDOne)

	assert.Equal(t, 1, len(got))
	assert.Equal(t, 2, len(got[0].Spans))
	assert.Equal(t, traceIDOne, got[0].Spans[0].TraceID)
	assert.Equal(t, clickhousestore.TestDataSpanNameOne, got[0].Spans[0].OperationName)
	assert.Equal(t, time.Duration(3600), got[0].Spans[0].Duration)
	assert.Equal(t, 2, len(got[0].Spans[0].Tags))
	assert.Contains(t, got[0].Spans[0].Tags[0].Key, "attr")
	assert.Contains(t, got[0].Spans[0].Tags[0].Value(), "value")
	assert.Contains(t, got[0].Spans[0].Tags[1].Key, "attr")
	assert.Contains(t, got[0].Spans[0].Tags[1].Value(), "value")
}
