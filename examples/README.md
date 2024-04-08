# Local Example

This example creates the following components:

- Clickhouse server
- OpenTelemetry Collector w/ clickhouseexporter defined
- Jaeger Query UI
- Jaeger Otel Clickhouse Backend

## Quickstart

From this directory run:

```shell
docker-compose up -d
```

Access the Jaeger UI at [http://localhost:16686](http://localhost:16686).

On first load, there will be no traces, but that will cause the backend to create and send traces to the collector. On refresh, you should see the backend service in the UI.
