// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package info

import (
	"fmt"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
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

var _ metrics.StatsClient = (*testStatsClient)(nil)

type testStatsClient struct {
	counts int64
}

func (ts *testStatsClient) Gauge(name string, value float64, tags []string, rate float64) error {
	return nil
}

func (ts *testStatsClient) Count(name string, value int64, tags []string, rate float64) error {
	atomic.AddInt64(&ts.counts, 1)
	return nil
}

func (ts *testStatsClient) Histogram(name string, value float64, tags []string, rate float64) error {
	return nil
}

func (ts *testStatsClient) Timing(name string, value time.Duration, tags []string, rate float64) error {
	return nil
}

func (ts *testStatsClient) Flush() error { return nil }

func TestReceiverStats(t *testing.T) {
	statsclient := &testStatsClient{}
	defer func(old metrics.StatsClient) { metrics.Client = statsclient }(metrics.Client)
	metrics.Client = statsclient

	tags := Tags{
		Lang:            "go",
		LangVersion:     "1.12",
		LangVendor:      "gov",
		Interpreter:     "gcc",
		TracerVersion:   "1.33",
		EndpointVersion: "v0.4",
	}
	testStats := func() *ReceiverStats {
		return &ReceiverStats{
			Stats: map[Tags]*TagStats{
				tags: {
					Tags: tags,
					Stats: Stats{
						TracesReceived:     1,
						TracesDropped:      &TracesDropped{1, 2, 3, 4, 5, 6, 7, 8},
						SpansMalformed:     &SpansMalformed{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
						TracesFiltered:     4,
						TracesPriorityNone: 5,
						TracesPerSamplingPriority: samplingPriorityStats{
							[maxAbsPriority*2 + 1]int64{
								maxAbsPriority + 0: 1,
								maxAbsPriority + 1: 2,
								maxAbsPriority + 2: 3,
								maxAbsPriority + 3: 4,
								maxAbsPriority + 4: 5,
							},
						},
						ClientDroppedP0Traces: 7,
						ClientDroppedP0Spans:  8,
						TracesBytes:           9,
						SpansReceived:         10,
						SpansDropped:          11,
						SpansFiltered:         12,
						EventsExtracted:       13,
						EventsSampled:         14,
						PayloadAccepted:       15,
						PayloadRefused:        16,
					},
				},
			},
		}
	}

	t.Run("Publish", func(t *testing.T) {
		testStats().Publish()
		assert.EqualValues(t, atomic.LoadInt64(&statsclient.counts), 39)
	})

	t.Run("reset", func(t *testing.T) {
		rcvstats := testStats()
		rcvstats.Reset()
		for _, tagstats := range rcvstats.Stats {
			stats := tagstats.Stats
			all := reflect.ValueOf(stats)
			for i := 0; i < all.NumField(); i++ {
				v := all.Field(i)
				if v.Kind() == reflect.Ptr {
					v = v.Elem()
				}
				assert.True(t, v.IsZero(), fmt.Sprintf("field %q not reset", all.Type().Field(i).Name))
			}
		}
	})

	t.Run("update", func(t *testing.T) {
		stats := NewReceiverStats()
		newstats := testStats()
		stats.Acc(newstats)
		assert.EqualValues(t, stats, newstats)
	})
}
