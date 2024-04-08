FROM golang:1.22-alpine AS build
WORKDIR /
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -gcflags "all=-N -l" -o ./jaeger-otel-clickhouse-backend

FROM scratch

COPY --from=build /jaeger-otel-clickhouse-backend /jaeger-otel-clickhouse-backend

ENTRYPOINT ["/jaeger-otel-clickhouse-backend"]
