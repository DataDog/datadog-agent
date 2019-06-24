package info

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestTracesDroppedStatsToMap(t *testing.T) {
	var s TracesDroppedStats
	s.DecodingError += 1
	m, err := s.toMap()
	assert.NoError(t, err)
	assert.EqualValues(t, m["decoding_error"], 1)
	// all other keys should be empty
	for k, v := range m {
		if k != "decoding_error" {
			assert.EqualValues(t, v, 0)
		}
	}
}

func TestTracesMalformedStatsToMap(t *testing.T) {
	var s TracesMalformedStats
	s.DuplicateSpanId += 1
	m, err := s.toMap()
	assert.NoError(t, err)
	assert.EqualValues(t, m["duplicate_span_id"], 1)
	// all other keys should be empty
	for k, v := range m {
		if k != "duplicate_span_id" {
			assert.EqualValues(t, v, 0)
		}
	}
}
