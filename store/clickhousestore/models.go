package clickhousestore

import "time"

type ClickhouseOtelSpan struct {
	Timestamp          time.Time           `db:"Timestamp"`
	TraceId            string              `db:"TraceId"`
	SpanId             string              `db:"SpanId"`
	ParentSpanId       string              `db:"ParentSpanId"`
	TraceState         string              `db:"TraceState"`
	SpanName           string              `db:"SpanName"`
	SpanKind           string              `db:"SpanKind"`
	ServiceName        string              `db:"ServiceName"`
	ResourceAttributes map[string]string   `db:"ResourceAttributes"`
	ScopeName          string              `db:"ScopeName"`
	ScopeVersion       string              `db:"ScopeVersion"`
	SpanAttributes     map[string]string   `db:"SpanAttributes"`
	Duration           int64               `db:"Duration"`
	StatusCode         string              `db:"StatusCode"`
	StatusMessage      string              `db:"StatusMessage"`
	EventsTimestamp    []time.Time         `db:"Events.Timestamp"`
	EventsName         []string            `db:"Events.Name"`
	EventsAttributes   []map[string]string `db:"Events.Attributes"`
}
