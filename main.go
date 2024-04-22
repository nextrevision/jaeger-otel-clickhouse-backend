package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"flag"
	"fmt"
	clickhouse "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/jaegertracing/jaeger/plugin/storage/grpc/shared"
	store "github.com/nextrevision/jaeger-otel-clickhouse-backend/store"
	"github.com/nextrevision/jaeger-otel-clickhouse-backend/store/clickhousestore"
	slogotel "github.com/remychantenay/slog-otel"
	"github.com/spf13/viper"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"log/slog"
	"net"
	"os"
	"strings"
	"time"
)

var tracer trace.Tracer

func newExporter(ctx context.Context, enabled bool) (sdktrace.SpanExporter, error) {
	if enabled {
		return otlptracegrpc.New(ctx)
	}
	return tracetest.NewNoopExporter(), nil
}

func newTraceProvider(exp sdktrace.SpanExporter) (*sdktrace.TracerProvider, error) {
	r, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("jaeger-otel-clickhouse-backend"),
		),
	)

	if err != nil {
		return nil, err
	}

	return sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(r),
	), nil
}

func initDB(cfg *store.Config) (*sql.DB, error) {
	var conn *sql.DB

	options := clickhouse.Options{
		Addr: []string{fmt.Sprintf("%s:%d", cfg.DBHost, cfg.DBPort)},
		Auth: clickhouse.Auth{
			Database: cfg.DBName,
			Username: cfg.DBUser,
			Password: cfg.DBPass,
		},
		Compression: &clickhouse.Compression{
			Method: clickhouse.CompressionLZ4,
		},
	}

	if cfg.DBTlsEnabled {
		options.TLS = &tls.Config{}
		if cfg.DBTlsInsecure {
			options.TLS.InsecureSkipVerify = true
		}

		if cfg.DBCaFile != "" {
			caCert, err := os.ReadFile(cfg.DBCaFile)
			if err != nil {
				return nil, err
			}
			caCertPool := x509.NewCertPool()
			caCertPool.AppendCertsFromPEM(caCert)
			options.TLS.RootCAs = caCertPool
		}
	}

	conn = clickhouse.OpenDB(&options)

	if cfg.DBMaxOpenConns != 0 {
		conn.SetMaxOpenConns(int(cfg.DBMaxOpenConns))
	}
	if cfg.DBMaxIdleConns != 0 {
		conn.SetMaxIdleConns(int(cfg.DBMaxIdleConns))
	}
	if cfg.DBConnMaxLifetimeMillis != 0 {
		conn.SetConnMaxLifetime(time.Millisecond * time.Duration(cfg.DBConnMaxLifetimeMillis))
	}
	if cfg.DBConnMaxIdleTimeMillis != 0 {
		conn.SetConnMaxIdleTime(time.Millisecond * time.Duration(cfg.DBConnMaxIdleTimeMillis))
	}

	if err := conn.Ping(); err != nil {
		return nil, err
	}
	return conn, nil
}

func main() {
	ctx := context.Background()

	// Set structured contextual logger
	slog.SetDefault(slog.New(slogotel.OtelHandler{
		Next: slog.NewJSONHandler(os.Stdout, nil),
	}).With("service", "jaeger-otel-clickhouse-backend"))

	logger := slog.Default()

	// Add flag for config
	var configPath string
	flag.StringVar(&configPath, "config", "", "A path to the yaml config file")
	flag.Parse()

	// Read config from viper
	v := viper.New()
	v.SetEnvPrefix("JOCB")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))

	if configPath != "" {
		v.SetConfigFile(configPath)

		err := v.ReadInConfig()
		if err != nil {
			logger.ErrorContext(ctx, "failed to parse configuration file", "error", err)
			os.Exit(1)
		}
	}

	// Initialize config
	cfg, err := store.NewConfig(v)
	if err != nil {
		logger.ErrorContext(ctx, "failed to parse configuration file", "error", err)
		os.Exit(1)
	}

	// Initialize tracing
	exp, err := newExporter(ctx, cfg.EnableTracing)
	if err != nil {
		logger.ErrorContext(ctx, "failed to initialize exporter", "error", err)
		os.Exit(1)
	}

	// Create a new tracer provider with a batch span processor and the given exporter.
	tp, err := newTraceProvider(exp)
	if err != nil {
		logger.ErrorContext(ctx, "unable to create trace provider", "error", err)
		os.Exit(1)
	}

	// Handle shutdown properly so nothing leaks.
	defer func() { _ = tp.Shutdown(ctx) }()

	otel.SetTracerProvider(tp)
	tracer = tp.Tracer("jaeger-otel-clickhouse-backend")

	// Initialize clickhouse db connection
	db, err := initDB(cfg)
	if err != nil {
		logger.ErrorContext(ctx, "unable to create clickhouse connection", "error", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()

	clickhouseStore := clickhousestore.New(cfg.DBTable, cfg.PadTraceID, db, tracer)

	// Create new storeBackend
	storeBackend := store.New(clickhouseStore, tracer)

	// Register store backend
	handler := shared.NewGRPCHandlerWithPlugins(storeBackend, nil, storeBackend)

	// Start gRPC server
	lis, err := net.Listen("tcp", ":14482")
	if err != nil {
		logger.ErrorContext(ctx, "failed to listen", "error", err)
	}

	server := grpc.NewServer()
	err = handler.Register(server)
	if err != nil {
		logger.ErrorContext(ctx, "unable to register server with grpc handler", "error", err)
		os.Exit(1)
	}

	logger.InfoContext(ctx, "server listening", "address", lis.Addr().String())
	if err := server.Serve(lis); err != nil {
		logger.ErrorContext(ctx, "failed to serve", "error", err)
		os.Exit(1)
	}
}
