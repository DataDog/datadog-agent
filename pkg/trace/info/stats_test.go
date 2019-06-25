package info

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestTracesDroppedStatsToMap(t *testing.T) {
	var s TracesDroppedStats
	s.DecodingError++
	m := s.tagValues()
	for k, v := range m {
		if k == "decoding_error" {
			assert.EqualValues(t, v, 1)
		} else {
			assert.EqualValues(t, v, 0)
		}
	}
}

func TestTracesMalformedStatsToMap(t *testing.T) {
	var s TracesMalformedStats
	s.DuplicateSpanID++
	m := s.tagValues()
	for k, v := range m {
		if k == "duplicate_span_id" {
			assert.EqualValues(t, v, 1)
		} else {
			assert.EqualValues(t, v, 0)
		}
	}
}

func TestTracesDroppedStatsToString(t *testing.T) {
	var s TracesDroppedStats
	s.DecodingError++
	s.ForeignSpan++
	assert.Equal(t, "decoding_error:1, foreign_span:1", s.String())
}

func TestTracesMalformedStatsToString(t *testing.T) {
	var s TracesMalformedStats
	s.ResourceEmpty++
	s.DuplicateSpanID++
	assert.Equal(t, "duplicate_span_id:1, resource_empty:1", s.String())
}
