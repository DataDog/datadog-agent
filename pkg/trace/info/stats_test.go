// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package info

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/log"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"

	"github.com/stretchr/testify/assert"
)

func TestTracesDropped(t *testing.T) {
	s := TracesDropped{}
	s.DecodingError.Store(1)
	s.ForeignSpan.Store(1)
	s.TraceIDZero.Store(1)
	s.SpanIDZero.Store(1)

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
	s := SpansMalformed{}
	s.ServiceEmpty.Store(1)
	s.ResourceEmpty.Store(1)
	s.ServiceInvalid.Store(1)
	s.SpanNameTruncate.Store(1)
	s.TypeTruncate.Store(1)

	t.Run("tagValues", func(t *testing.T) {
		assert.Equal(t, map[string]int64{
			"span_name_invalid":        0,
			"span_name_empty":          0,
			"service_truncate":         0,
			"peer_service_truncate":    0,
			"peer_service_invalid":     0,
			"invalid_start_date":       0,
			"invalid_http_status_code": 0,
			"invalid_duration":         0,
			"duplicate_span_id":        0,
			"service_empty":            1,
			"resource_empty":           1,
			"service_invalid":          1,
			"span_name_truncate":       1,
			"type_truncate":            1,
			"invalid_span_links":	    1,
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
	s := samplingPriorityStats{}
	s.counts[0].Store(1)
	s.counts[10].Store(2)
	s.counts[20].Store(3)

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

	s3 := samplingPriorityStats{}
	s3.counts[0].Store(1)
	s3.counts[10].Store(2)
	s3.counts[20].Store(3)
	s4 := samplingPriorityStats{}
	s4.counts[1].Store(1)
	s4.counts[10].Store(5)
	s4.counts[19].Store(10)
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
	counts atomic.Int64
}

//nolint:revive // TODO(APM) Fix revive linter
func (ts *testStatsClient) Gauge(name string, value float64, tags []string, rate float64) error {
	return nil
}

//nolint:revive // TODO(APM) Fix revive linter
func (ts *testStatsClient) Count(name string, value int64, tags []string, rate float64) error {
	ts.counts.Inc()
	return nil
}

//nolint:revive // TODO(APM) Fix revive linter
func (ts *testStatsClient) Histogram(name string, value float64, tags []string, rate float64) error {
	return nil
}

//nolint:revive // TODO(APM) Fix revive linter
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
		stats := Stats{}
		stats.TracesReceived.Store(1)
		stats.TracesFiltered.Store(4)
		stats.TracesPriorityNone.Store(5)
		stats.TracesPerSamplingPriority = samplingPriorityStats{}
		stats.TracesPerSamplingPriority.counts[maxAbsPriority+0].Store(1)
		stats.TracesPerSamplingPriority.counts[maxAbsPriority+1].Store(2)
		stats.TracesPerSamplingPriority.counts[maxAbsPriority+2].Store(3)
		stats.TracesPerSamplingPriority.counts[maxAbsPriority+3].Store(4)
		stats.TracesPerSamplingPriority.counts[maxAbsPriority+4].Store(5)
		stats.ClientDroppedP0Traces.Store(7)
		stats.ClientDroppedP0Spans.Store(8)
		stats.TracesBytes.Store(9)
		stats.SpansReceived.Store(10)
		stats.SpansDropped.Store(11)
		stats.SpansFiltered.Store(12)
		stats.EventsExtracted.Store(13)
		stats.EventsSampled.Store(14)
		stats.PayloadAccepted.Store(15)
		stats.PayloadRefused.Store(16)
		stats.TracesDropped = &TracesDropped{}
		stats.TracesDropped.DecodingError.Store(1)
		stats.TracesDropped.PayloadTooLarge.Store(2)
		stats.TracesDropped.EmptyTrace.Store(3)
		stats.TracesDropped.TraceIDZero.Store(4)
		stats.TracesDropped.SpanIDZero.Store(5)
		stats.TracesDropped.ForeignSpan.Store(6)
		stats.TracesDropped.Timeout.Store(7)
		stats.TracesDropped.EOF.Store(8)
		stats.SpansMalformed = &SpansMalformed{}
		stats.SpansMalformed.DuplicateSpanID.Store(1)
		stats.SpansMalformed.ServiceEmpty.Store(2)
		stats.SpansMalformed.ServiceTruncate.Store(3)
		stats.SpansMalformed.ServiceInvalid.Store(4)
		stats.SpansMalformed.SpanNameEmpty.Store(5)
		stats.SpansMalformed.SpanNameTruncate.Store(6)
		stats.SpansMalformed.PeerServiceTruncate.Store(7)
		stats.SpansMalformed.PeerServiceInvalid.Store(8)
		stats.SpansMalformed.SpanNameInvalid.Store(9)
		stats.SpansMalformed.ResourceEmpty.Store(10)
		stats.SpansMalformed.TypeTruncate.Store(11)
		stats.SpansMalformed.InvalidStartDate.Store(12)
		stats.SpansMalformed.InvalidDuration.Store(13)
		stats.SpansMalformed.InvalidHTTPStatusCode.Store(14)
		stats.SpansMalformed.InvalidHTTPStatusCode.Store(15)
		return &ReceiverStats{
			Stats: map[Tags]*TagStats{
				tags: {
					Tags:  tags,
					Stats: stats,
				},
			},
		}
	}

	t.Run("PublishAndReset", func(t *testing.T) {
		rs := testStats()
		rs.PublishAndReset()
		assert.EqualValues(t, 41, statsclient.counts.Load())
		assertStatsAreReset(t, rs)
	})

	t.Run("update", func(t *testing.T) {
		stats := NewReceiverStats()
		newstats := testStats()
		stats.Acc(newstats)
		assert.EqualValues(t, stats, newstats)
	})

	t.Run("LogAndResetStats", func(t *testing.T) {
		var b bytes.Buffer
		log.SetLogger(log.NewBufferLogger(&b))

		rs := testStats()
		rs.LogAndResetStats()

		log.Flush()
		logs := strings.Split(b.String(), "\n")
		assert.Equal(t, "[INFO] [lang:go lang_version:1.12 lang_vendor:gov interpreter:gcc tracer_version:1.33 endpoint_version:v0.4] -> traces received: 1, traces filtered: 4, traces amount: 9 bytes, events extracted: 13, events sampled: 14",
			logs[0])
		assert.Equal(t, "[WARN] [lang:go lang_version:1.12 lang_vendor:gov interpreter:gcc tracer_version:1.33 endpoint_version:v0.4] -> traces_dropped(decoding_error:1, empty_trace:3, foreign_span:6, payload_too_large:2, span_id_zero:5, timeout:7, trace_id_zero:4, unexpected_eof:8), spans_malformed(duplicate_span_id:1, invalid_duration:13, invalid_http_status_code:14, invalid_start_date:12, peer_service_invalid:8, peer_service_truncate:7, resource_empty:10, service_empty:2, service_invalid:4, service_truncate:3, span_name_empty:5, span_name_invalid:9, span_name_truncate:6, type_truncate:11). Enable debug logging for more details.",
			logs[1])

		assertStatsAreReset(t, rs)
	})
}

func assertStatsAreReset(t *testing.T, rs *ReceiverStats) {
	for _, tagstats := range rs.Stats {
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
}
