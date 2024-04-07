# Jaeger OpenTelemetry Clickhouse Backend 

This project is a Jaeger gRPC backend (v1) compatible with the [OpenTelemetry Clickhouse exporter](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/exporter/clickhouseexporter). It provides a way of visualizing trace data via the Jaeger Query frontend.

## Quickstart

Requirements:
- A running clickhouse instance
- OpenTelemetry Collector exporting to Clickhouse via the clickhouseexporter plugin

Start the backend:

```
go run main.go -config my-config.yaml
```

Start Jaeger Query:

```
docker run -p 16686:16686 -e SPAN_STORAGE_TYPE=grpc-plugin jaegertracing/jaeger-query:1.56.0 --grpc-storage.server localhost:14482
```

## Config

Can be set by YAML file and the `-config` flag or by environment variable with the `JOC` prefix.

| Env Var                            | YAML                           | Type   | Required | Default       | Example           |
|------------------------------------|--------------------------------|--------|----------|---------------|-------------------|
| `JOC_DB_HOST`                      | `db_host`                      | string | true     |               | `127.0.0.1`       |
| `JOC_DB_PORT`                      | `db_port`                      | int    | true     |               | `9000`            |
| `JOC_DB_USER`                      | `db_user`                      | string | true     |               | `test_user`       |
| `JOC_DB_PASS`                      | `db_pass`                      | string | true     |               | `test_pass`       |
| `JOC_DB_NAME`                      | `db_name`                      | string | true     | `otel`        | `custom_database` |
| `JOC_DB_TABLE`                     | `db_table`                     | string | true     | `otel_traces` | `trace_data`      |
| `JOC_DB_CA_FILE`                   | `db_ca_file`                   | string | false    |               | `/ca.crt`         |
| `JOC_DB_TLS_ENABLED`               | `db_tls_enabled`               | bool   | false    | `false`       | `true`            |
| `JOC_DB_TLS_INSECURE`              | `db_tls_insecure`              | bool   | false    | `false`       | `true`            |
| `JOC_DB_MAX_OPEN_CONNS`            | `db_max_open_conns`            | int    | false    |               | `10`              |
| `JOC_DB_MAX_IDLE_CONNS`            | `db_max_idle_conns`            | int    | false    |               | `5`               |
| `JOC_DB_CONN_MAX_LIFETIME_MILLIS`  | `db_conn_max_lifetime_millis`  | int    | false    |               | `3000`            |
| `JOC_DB_CONN_MAX_IDLE_TIME_MILLIS` | `db_conn_max_idle_time_millis` | int    | false    |               | `1000`            |
| `JOC_ENABLE_TRACING`               | `enable_tracing`               | bool   | false    | `false`        | `true`            |

### Tracing

The backend has been instrumented with OpenTelemetry and can be configured to export traces via gRPC to an OTLP compatible endpoint. This can be enabled using the `JOC_ENABLE_TRACING=true` environment variable and setting `OTEL_EXPORTER_OTLP_ENDPOINT` to the desired OTLP compatible address.

## Tag Search Syntax

I took the liberty to enhance the tag search expressivity with wildcards and regex patterns.

### Wildcards

In the "Tags" field, using a `%` character will result in a wildcard match using SQL `LIKE` grammar. The following is an example of a tag query and the resulting SQL:

```sql
# http.url=http%://duckduckgo.com
  
SELECT ... WHERE SpanAttributes['http.url'] LIKE 'http%://duckduckgo.com'
```

### Regex

In the "Tags" field, using the operator `=~` character will result in a regex match using the Clickhouse [match](https://clickhouse.com/docs/en/sql-reference/functions/string-search-functions#match) function. The following is an example of a tag query and the resulting SQL:

```sql
# http.url=~http://[duck]+go.com
  
SELECT ... WHERE match(SpanAttributes['http.url'], 'http://[duck]+go.com')
```
