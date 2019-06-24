package info

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestTracesDroppedStatsToMap(t *testing.T) {
	var s TracesDroppedStats
	s.DecodingError += 1
	m := s.toMap()
	assert.EqualValues(t, m["decoding_error"], 1)
	assert.Len(t, m, 1, "stats map should contain exactly one item as count=0 stats should be omitted")
}

func TestTracesMalformedStatsToMap(t *testing.T) {
	var s TracesMalformedStats
	s.DuplicateSpanId += 1
	m := s.toMap()
	assert.EqualValues(t, m["duplicate_span_id"], 1)
	assert.Len(t, m, 1, "stats map should contain exactly one item as count=0 stats should be omitted")
}
