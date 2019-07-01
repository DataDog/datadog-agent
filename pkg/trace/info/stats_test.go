package info

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTracesDropped(t *testing.T) {
	s := TracesDropped{
		DecodingError: 1,
		ForeignSpan:   1,
		TraceIDZero:   1,
		SpanIDZero:    1,
	}

	t.Run("tagValues", func(t *testing.T) {
		expected := (&TracesDropped{}).tagValues()
		expected["decoding_error"] = 1
		expected["foreign_span"] = 1
		expected["trace_id_zero"] = 1
		expected["span_id_zero"] = 1
		assert.Equal(t, expected, s.tagValues())
	})

	t.Run("String", func(t *testing.T) {
		assert.Equal(t, "decoding_error:1, foreign_span:1, span_id_zero:1, trace_id_zero:1", s.String())
	})
}

func TestSpansMalformed(t *testing.T) {
	s := SpansMalformed{
		ServiceEmpty:     1,
		ResourceEmpty:    1,
		ServiceInvalid:   1,
		SpanNameTruncate: 1,
		TypeTruncate:     1,
	}

	t.Run("tagValues", func(t *testing.T) {
		expected := (&SpansMalformed{}).tagValues()
		expected["service_empty"] = 1
		expected["resource_empty"] = 1
		expected["service_invalid"] = 1
		expected["span_name_truncate"] = 1
		expected["type_truncate"] = 1
		assert.Equal(t, expected, s.tagValues())
	})

	t.Run("String", func(t *testing.T) {
		assert.Equal(t, "resource_empty:1, service_empty:1, service_invalid:1, span_name_truncate:1, type_truncate:1", s.String())
	})
}
