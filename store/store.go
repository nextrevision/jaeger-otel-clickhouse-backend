package store

import (
	"context"
	"database/sql"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	"github.com/nextrevision/jaeger-otel-clickhouse-backend/store/clickhousestore"
	"go.opentelemetry.io/otel/trace"
)

type Store struct {
	db     *sql.DB
	table  string
	reader *clickhousestore.ClickhouseReader
	tracer trace.Tracer
}

func New(cfg *Config, db *sql.DB, tracer trace.Tracer) *Store {
	return &Store{
		db:     db,
		table:  cfg.DBTable,
		reader: clickhousestore.New(cfg.DBTable, db, tracer),
		tracer: tracer,
	}
}

func (s *Store) SpanReader() spanstore.Reader {
	return s.reader
}

func (s *Store) DependencyReader() dependencystore.Reader {
	return s.reader
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
