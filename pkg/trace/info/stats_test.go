// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

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
		assert.Equal(t, map[string]int64{
			"empty_trace":    0,
			"decoding_error": 1,
			"foreign_span":   1,
			"trace_id_zero":  1,
			"span_id_zero":   1,
		}, s.tagValues())
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
		assert.Equal(t, map[string]int64{
			"span_name_invalid":  0,
			"span_name_empty":    0,
			"service_truncate":   0,
			"invalid_start_date": 0,
			"invalid_duration":   0,
			"duplicate_span_id":  0,
			"service_empty":      1,
			"resource_empty":     1,
			"service_invalid":    1,
			"span_name_truncate": 1,
			"type_truncate":      1,
		}, s.tagValues())
	})

	t.Run("String", func(t *testing.T) {
		assert.Equal(t, "resource_empty:1, service_empty:1, service_invalid:1, span_name_truncate:1, type_truncate:1", s.String())
	})
}
