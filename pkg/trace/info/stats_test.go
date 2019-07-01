package info

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTracesDropped(t *testing.T) {
	s := TracesDropped{
		DecodingError: 1,
		ForeignSpan: 1,
	}

	t.Run("StatsToMap", func(t *testing.T) {
		expected := (&TracesDropped{}).tagValues()
		expected["decoding_error"] = 1
		expected["foreign_span"] = 1
		assert.Equal(t, expected, s.tagValues())
	})

	t.Run("StatsToString", func(t *testing.T) {
		assert.Equal(t, "decoding_error:1, foreign_span:1", s.String())
	})
}

func TestSpansMalformed(t *testing.T) {
	s := SpansMalformed{
		ServiceEmpty:1,
		ResourceEmpty:1,
	}

	t.Run("StatsToMap", func(t *testing.T) {
		expected := (&SpansMalformed{}).tagValues()
		expected["service_empty"] = 1
		expected["resource_empty"] = 1
		assert.Equal(t, expected, s.tagValues())
	})

	t.Run("StatsToString", func(t *testing.T) {
		assert.Equal(t, "resource_empty:1, service_empty:1", s.String())
	})
}
