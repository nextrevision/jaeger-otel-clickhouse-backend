package clickhousestore

import (
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/trace"
	"testing"
)

func TestClickhouseReader_padTraceIDs(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	tracer := trace.NewNoopTracerProvider().Tracer("test-tracer")

	cr := New("test", true, db, tracer)
	res := cr.padTraceIDs([]string{"c91fd0eb7e1193f8", "c91fd0eb7e1193f9"})
	assert.Equal(t, []string{"0000000000000000c91fd0eb7e1193f8", "0000000000000000c91fd0eb7e1193f9"}, res)
}
