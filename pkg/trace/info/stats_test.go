package info

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestTracesDropped(t *testing.T) {
	var s TracesDropped
	s.DecodingError++
	s.ForeignSpan++

	t.Run("StatsToMap", func(t *testing.T) {
		for k, v := range s.TagValues() {
			if k == "decoding_error" || k == "foreign_span" {
				assert.EqualValues(t, v, 1)
			} else {
				assert.EqualValues(t, v, 0)
			}
		}
	})

	t.Run("StatsToString", func(t *testing.T) {
		assert.Equal(t, "decoding_error:1, foreign_span:1", s.String())
	})
}

func TestTracesMalformed(t *testing.T) {
	var s TracesMalformed
	s.ServiceEmpty++
	s.ResourceEmpty++

	t.Run("StatsToMap", func(t *testing.T) {
		for k, v := range s.TagValues() {
			if k == "service_empty" || k == "resource_empty" {
				assert.EqualValues(t, v, 1)
			} else {
				assert.EqualValues(t, v, 0)
			}
		}
	})

	t.Run("StatsToString", func(t *testing.T) {
		assert.Equal(t, "resource_empty:1, service_empty:1", s.String())
	})
}
