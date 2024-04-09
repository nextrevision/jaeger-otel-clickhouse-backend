package clickhousestore

import (
	"time"
)

type ClickhouseOtelTrace struct {
	TraceID string
	Spans   []ClickhouseOtelSpan
}

type ClickhouseOtelSpan struct {
	Timestamp          time.Time
	TraceID            string
	SpanID             string
	ParentSpanID       string
	TraceState         string
	SpanName           string
	SpanKind           string
	ServiceName        string
	ResourceAttributes map[string]string
	ScopeName          string
	ScopeVersion       string
	SpanAttributes     map[string]string
	Duration           int64
	StatusCode         string
	StatusMessage      string
	EventsTimestamp    []time.Time
	EventsName         []string
	EventsAttributes   []map[string]string
}

type SearchOptions struct {
	SpanName        string
	Attributes      map[string]string
	IgnoredTraceIDs []string
	MinDuration     time.Duration
	MaxDuration     time.Duration
	SearchLimit     int
}
