package clickhousestore

import (
	"context"
	"time"
)

const (
	TestDataTraceIDOne     = "843bc5b94cbaa733844dfe41f33167ad"
	TestDataTraceIDTwo     = "373ee1ef9f1f5f2abc5d900ddc7e94ef"
	TestDataServiceNameOne = "test-client"
	TestDataServiceNameTwo = "test-server"
	TestDataSpanNameOne    = "parent-span"
	TestDataSpanNameTwo    = "child-span"
)

type MockClickhouseReader struct {
	returnCount int
	traces      map[string]*ClickhouseOtelTrace
}

func NewMockClickhouseReader(returnCount int) *MockClickhouseReader {
	return &MockClickhouseReader{
		returnCount: returnCount,
		traces: map[string]*ClickhouseOtelTrace{
			TestDataTraceIDOne: &ClickhouseOtelTrace{
				TraceID: TestDataTraceIDOne,
				Spans: []ClickhouseOtelSpan{
					{
						Timestamp:    time.Time{},
						TraceID:      TestDataTraceIDOne,
						SpanID:       "a7d2aa025caa9cb8",
						ParentSpanID: "",
						TraceState:   "",
						SpanName:     TestDataSpanNameOne,
						SpanKind:     "client",
						ServiceName:  TestDataServiceNameOne,
						ResourceAttributes: map[string]string{
							"resource-1": "value-1",
							"resource-2": "value-2",
						},
						ScopeName:    TestDataServiceNameOne,
						ScopeVersion: "",
						SpanAttributes: map[string]string{
							"attr-1": "value-1",
							"attr-2": "value-2",
						},
						Duration:         3600,
						StatusCode:       "STATUS_CODE_UNSET",
						StatusMessage:    "",
						EventsTimestamp:  nil,
						EventsName:       nil,
						EventsAttributes: nil,
					},
					{
						Timestamp:          time.Time{},
						TraceID:            TestDataTraceIDOne,
						SpanID:             "0d8fd33795ba49aa",
						ParentSpanID:       "a7d2aa025caa9cb8",
						TraceState:         "",
						SpanName:           TestDataSpanNameTwo,
						SpanKind:           "client",
						ServiceName:        TestDataServiceNameOne,
						ResourceAttributes: nil,
						ScopeName:          TestDataServiceNameOne,
						ScopeVersion:       "",
						SpanAttributes:     nil,
						Duration:           3600,
						StatusCode:         "STATUS_CODE_UNSET",
						StatusMessage:      "",
						EventsTimestamp:    nil,
						EventsName:         nil,
						EventsAttributes:   nil,
					},
				},
			},
			TestDataTraceIDTwo: &ClickhouseOtelTrace{
				TraceID: TestDataTraceIDTwo,
				Spans: []ClickhouseOtelSpan{
					{
						Timestamp:    time.Time{},
						TraceID:      TestDataTraceIDTwo,
						SpanID:       "a7d2aa025caa9cb8",
						ParentSpanID: "",
						TraceState:   "",
						SpanName:     TestDataSpanNameOne,
						SpanKind:     "server",
						ServiceName:  TestDataServiceNameTwo,
						ResourceAttributes: map[string]string{
							"resource-1": "value-1",
							"resource-2": "value-2",
						},
						ScopeName:    TestDataServiceNameTwo,
						ScopeVersion: "",
						SpanAttributes: map[string]string{
							"attr-1": "value-1",
							"attr-2": "value-2",
						},
						Duration:         3600,
						StatusCode:       "STATUS_CODE_UNSET",
						StatusMessage:    "",
						EventsTimestamp:  nil,
						EventsName:       nil,
						EventsAttributes: nil,
					},
					{
						Timestamp:          time.Time{},
						TraceID:            TestDataTraceIDTwo,
						SpanID:             "0d8fd33795ba49aa",
						ParentSpanID:       "a7d2aa025caa9cb8",
						TraceState:         "",
						SpanName:           TestDataSpanNameTwo,
						SpanKind:           "server",
						ServiceName:        TestDataServiceNameTwo,
						ResourceAttributes: nil,
						ScopeName:          TestDataServiceNameTwo,
						ScopeVersion:       "",
						SpanAttributes:     nil,
						Duration:           3600,
						StatusCode:         "STATUS_CODE_UNSET",
						StatusMessage:      "",
						EventsTimestamp:    nil,
						EventsName:         nil,
						EventsAttributes:   nil,
					},
				},
			},
		},
	}
}

func (r *MockClickhouseReader) GetServices(ctx context.Context) ([]string, error) {
	if r.returnCount == 0 {
		return []string{}, nil
	} else if r.returnCount == 1 {
		return []string{TestDataServiceNameOne}, nil
	}
	return []string{TestDataServiceNameOne, TestDataServiceNameTwo}, nil
}

func (r *MockClickhouseReader) GetSpanNames(ctx context.Context, serviceName string) ([]string, error) {
	if r.returnCount == 0 {
		return []string{}, nil
	} else if r.returnCount == 1 {
		return []string{TestDataSpanNameOne}, nil
	}
	return []string{TestDataSpanNameOne, TestDataSpanNameTwo}, nil
}

func (r *MockClickhouseReader) GetTrace(ctx context.Context, traceID string) (*ClickhouseOtelTrace, error) {
	return r.traces[traceID], nil
}

func (r *MockClickhouseReader) GetTraces(ctx context.Context, traceIDs []string) ([]*ClickhouseOtelTrace, error) {
	traces := make([]*ClickhouseOtelTrace, 0, 2)

	if r.returnCount == 0 {
		return traces, nil
	} else if r.returnCount == 1 {
		traces = append(traces, r.traces[TestDataTraceIDOne])
		return traces, nil
	}

	for _, trace := range r.traces {
		traces = append(traces, trace)
	}
	return traces, nil
}

func (r *MockClickhouseReader) SearchTraces(ctx context.Context, serviceName string, startTime time.Time, endTime time.Time, options SearchOptions) ([]string, error) {
	if r.returnCount == 0 {
		return []string{}, nil
	} else if r.returnCount == 1 {
		return []string{TestDataTraceIDOne}, nil
	}
	return []string{TestDataTraceIDOne, TestDataTraceIDTwo}, nil
}
