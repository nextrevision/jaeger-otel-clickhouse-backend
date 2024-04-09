package store

import (
	"context"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	"github.com/nextrevision/jaeger-otel-clickhouse-backend/store/clickhousestore"
	"go.opentelemetry.io/otel/trace"
	"log/slog"
)

type Store struct {
	clickhousestore clickhousestore.ClickhouseStore
	tracer          trace.Tracer
	logger          *slog.Logger
}

func New(store clickhousestore.ClickhouseStore, tracer trace.Tracer) *Store {
	return &Store{
		clickhousestore: store,
		tracer:          tracer,
		logger:          slog.Default(),
	}
}

func (s *Store) SpanReader() spanstore.Reader {
	return s
}

func (s *Store) DependencyReader() dependencystore.Reader {
	return s
}

func (s *Store) SpanWriter() spanstore.Writer {
	return s
}

func (s *Store) StreamingSpanWriter() spanstore.Writer {
	return s
}

func (s *Store) Close() error {
	return nil
}

func (s *Store) WriteSpan(ctx context.Context, span *model.Span) error {
	return nil
}
