// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package info

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/sampler"

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
			"empty_trace":       0,
			"payload_too_large": 0,
			"decoding_error":    1,
			"foreign_span":      1,
			"trace_id_zero":     1,
			"span_id_zero":      1,
			"timeout":           0,
			"unexpected_eof":    0,
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
			"span_name_invalid":        0,
			"span_name_empty":          0,
			"service_truncate":         0,
			"invalid_start_date":       0,
			"invalid_http_status_code": 0,
			"invalid_duration":         0,
			"duplicate_span_id":        0,
			"service_empty":            1,
			"resource_empty":           1,
			"service_invalid":          1,
			"span_name_truncate":       1,
			"type_truncate":            1,
		}, s.tagValues())
	})

	t.Run("String", func(t *testing.T) {
		assert.Equal(t, "resource_empty:1, service_empty:1, service_invalid:1, span_name_truncate:1, type_truncate:1", s.String())
	})
}

func TestStatsTags(t *testing.T) {
	assert.Equal(t, (&Tags{
		Lang:            "go",
		LangVersion:     "1.14",
		LangVendor:      "gov",
		Interpreter:     "goi",
		TracerVersion:   "1.21.0",
		EndpointVersion: "v0.4",
	}).toArray(), []string{
		"lang:go",
		"lang_version:1.14",
		"lang_vendor:gov",
		"interpreter:goi",
		"tracer_version:1.21.0",
		"endpoint_version:v0.4",
	})
}

func TestSamplingPriorityStats(t *testing.T) {
	s := samplingPriorityStats{
		[21]int64{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3},
	}

	t.Run("TagValues", func(t *testing.T) {
		assert.Equal(t, map[string]int64{
			"-10": 1,
			"0":   2,
			"10":  3,
		}, s.TagValues())
	})

	t.Run("reset", func(t *testing.T) {
		s.reset()
		assert.Equal(t, map[string]int64{}, s.TagValues())
	})

	s2 := samplingPriorityStats{}
	t.Run("CountSamplingPriority", func(t *testing.T) {
		for i := -10; i <= 10; i++ {
			for j := 0; j <= i+10; j++ {
				s2.CountSamplingPriority(sampler.SamplingPriority(i))
			}
		}
		assert.Equal(t, map[string]int64{
			"-10": 1,
			"-9":  2,
			"-8":  3,
			"-7":  4,
			"-6":  5,
			"-5":  6,
			"-4":  7,
			"-3":  8,
			"-2":  9,
			"-1":  10,
			"0":   11,
			"1":   12,
			"2":   13,
			"3":   14,
			"4":   15,
			"5":   16,
			"6":   17,
			"7":   18,
			"8":   19,
			"9":   20,
			"10":  21,
		}, s2.TagValues())
	})

	s3 := samplingPriorityStats{
		[21]int64{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3},
	}
	s4 := samplingPriorityStats{
		[21]int64{0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 5, 0, 0, 0, 0, 0, 0, 0, 0, 10, 0},
	}
	t.Run("update", func(t *testing.T) {
		s3.update(&s4)
		assert.Equal(t, map[string]int64{
			"-10": 1,
			"-9":  1,
			"0":   7,
			"9":   10,
			"10":  3,
		}, s3.TagValues())
	})

}
