// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tracestats

import (
	"fmt"
	"math"
	"testing"

	stats "github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/metrics/tracestats/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"google.golang.org/protobuf/proto"
)

func sdkTraceMetric(unit string, count uint64, sum float64, attrs map[string]string) pmetric.Metric {
	metric := pmetric.NewMetric()
	metric.SetName(SDKTraceMetricName)
	metric.SetUnit(unit)
	histogram := metric.SetEmptyHistogram()
	histogram.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	datapoint := histogram.DataPoints().AppendEmpty()
	datapoint.SetStartTimestamp(1)
	datapoint.SetTimestamp(2)
	datapoint.SetCount(count)
	datapoint.SetSum(sum)
	datapoint.SetMin(0)
	datapoint.SetMax(sum)
	datapoint.BucketCounts().FromRaw([]uint64{count})
	for key, value := range attrs {
		datapoint.Attributes().PutStr(key, value)
	}
	return metric
}

func groupedByName(payload *stats.OTLPIntakeStatsPayload, name string) *stats.StatsBucketV3_GroupedStats {
	if payload == nil {
		return nil
	}
	for _, bucket := range payload.Stats {
		for _, groupedStats := range bucket.Stats {
			if groupedStats.Name == name {
				return groupedStats
			}
		}
	}
	return nil
}

func sparseSketch(t testing.TB, raw []byte) *stats.SparseSketch {
	t.Helper()
	require.NotEmpty(t, raw)
	sketch := &stats.SparseSketch{}
	require.NoError(t, proto.Unmarshal(raw, sketch))
	return sketch
}

func TestSpanKindFromAttr(t *testing.T) {
	tests := []struct {
		value    string
		expected ptrace.SpanKind
	}{
		{value: "SPAN_KIND_SERVER", expected: ptrace.SpanKindServer},
		{value: "server", expected: ptrace.SpanKindServer},
		{value: "SPAN_KIND_CLIENT", expected: ptrace.SpanKindClient},
		{value: "client", expected: ptrace.SpanKindClient},
		{value: "SPAN_KIND_PRODUCER", expected: ptrace.SpanKindProducer},
		{value: "producer", expected: ptrace.SpanKindProducer},
		{value: "SPAN_KIND_CONSUMER", expected: ptrace.SpanKindConsumer},
		{value: "consumer", expected: ptrace.SpanKindConsumer},
		{value: "SPAN_KIND_INTERNAL", expected: ptrace.SpanKindInternal},
		{value: "internal", expected: ptrace.SpanKindInternal},
		{value: "unknown", expected: ptrace.SpanKindUnspecified},
	}

	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			attrs := pcommon.NewMap()
			attrs.PutStr("span.kind", test.value)
			assert.Equal(t, test.expected, spanKindFromAttr(attrs))
		})
	}
}

func TestBuildSDKTraceStatsPayloadPreservesExplicitHistogram(t *testing.T) {
	metric := sdkTraceMetric("s", 6, 0.5, map[string]string{"datadog.operation.name": "op"})
	datapoint := metric.Histogram().DataPoints().At(0)
	datapoint.ExplicitBounds().FromRaw([]float64{0.01, 0.1})
	datapoint.BucketCounts().FromRaw([]uint64{1, 2, 3})

	payload, conversionErrors := BuildSDKTraceStatsPayload("", "", pcommon.NewMap(), metric)

	require.Empty(t, conversionErrors)
	groupedStats := groupedByName(payload, "op")
	require.NotNil(t, groupedStats)
	sketch := sparseSketch(t, groupedStats.OkSparseSketch)
	assert.Equal(t, []int32{-32768, 0, 1, 2, 3, 4}, sketch.K)
	assert.Equal(t, []uint32{2, math.Float32bits(0.01), math.Float32bits(0.1), 1, 2, 3}, sketch.N)
	assert.Equal(t, int64(6), sketch.Basic.Count)
	assert.InDelta(t, 0.5, sketch.Basic.Sum, 1e-12)
}

func TestBuildSDKTraceStatsPayloadSkipsInvalidDatapoints(t *testing.T) {
	metric := sdkTraceMetric("s", 1, 1, map[string]string{"datadog.operation.name": "valid"})
	invalid := metric.Histogram().DataPoints().AppendEmpty()
	invalid.SetCount(2)
	invalid.SetSum(1)
	invalid.BucketCounts().FromRaw([]uint64{1})
	invalid.Attributes().PutStr("datadog.operation.name", "invalid")

	payload, conversionErrors := BuildSDKTraceStatsPayload("", "", pcommon.NewMap(), metric)

	require.Len(t, conversionErrors, 1)
	assert.Equal(t, 1, conversionErrors[0].DataPointIndex)
	assert.ErrorContains(t, conversionErrors[0].Err, "count 2 mismatch total bins 1")
	require.Len(t, payload.Stats, 1)
	assert.NotNil(t, groupedByName(payload, "valid"))
	assert.Nil(t, groupedByName(payload, "invalid"))
}

func TestBuildSDKTraceStatsPayloadInputContract(t *testing.T) {
	for name, change := range map[string]func(pmetric.Metric){
		"cumulative": func(m pmetric.Metric) {
			m.Histogram().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
		},
		"unrelated":        func(m pmetric.Metric) { m.SetName("custom.duration") },
		"unsupported unit": func(m pmetric.Metric) { m.SetUnit("ms") },
		"no recorded value": func(m pmetric.Metric) {
			m.Histogram().DataPoints().At(0).SetFlags(pmetric.DefaultDataPointFlags.WithNoRecordedValue(true))
		},
		"missing start":     func(m pmetric.Metric) { m.Histogram().DataPoints().At(0).SetStartTimestamp(0) },
		"reversed interval": func(m pmetric.Metric) { m.Histogram().DataPoints().At(0).SetStartTimestamp(3) },
		"timestamp overflow": func(m pmetric.Metric) {
			m.Histogram().DataPoints().At(0).SetTimestamp(pcommon.Timestamp(math.MaxUint64))
		},
		"duration overflow": func(m pmetric.Metric) { m.Histogram().DataPoints().At(0).SetSum(math.MaxFloat64) },
		"missing counts":    func(m pmetric.Metric) { m.Histogram().DataPoints().At(0).BucketCounts().FromRaw(nil) },
		"negative bound": func(m pmetric.Metric) {
			dp := m.Histogram().DataPoints().At(0)
			dp.ExplicitBounds().FromRaw([]float64{-1})
			dp.BucketCounts().FromRaw([]uint64{0, 1})
		},
		"float32 collision": func(m pmetric.Metric) {
			dp := m.Histogram().DataPoints().At(0)
			dp.ExplicitBounds().FromRaw([]float64{1, math.Nextafter(1, 2)})
			dp.BucketCounts().FromRaw([]uint64{0, 0, 1})
		},
	} {
		t.Run(name, func(t *testing.T) {
			metric := sdkTraceMetric("s", 1, 1, nil)
			change(metric)
			payload, _ := BuildSDKTraceStatsPayload("", "", pcommon.NewMap(), metric)
			assert.Nil(t, payload)
		})
	}
}

func TestBuildSDKTraceStatsPayloadRejectsInvalidCounts(t *testing.T) {
	tests := []struct {
		name       string
		count      uint64
		counts     []uint64
		wantErrMsg string
	}{
		{name: "count exceeds signed wire range", count: uint64(math.MaxInt64) + 1, counts: []uint64{uint64(math.MaxInt64) + 1}, wantErrMsg: "exceeds maximum"},
		{name: "bucket exceeds sparse bin range", count: uint64(math.MaxUint32) + 1, counts: []uint64{uint64(math.MaxUint32) + 1}, wantErrMsg: "bucket count"},
		{name: "bucket sum overflows", count: 1, counts: []uint64{1, math.MaxUint64}, wantErrMsg: "bucket counts overflow"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			metric := sdkTraceMetric("s", test.count, 1, nil)
			dp := metric.Histogram().DataPoints().At(0)
			dp.BucketCounts().FromRaw(test.counts)
			if len(test.counts) > 1 {
				dp.ExplicitBounds().FromRaw(make([]float64, len(test.counts)-1))
			}

			payload, conversionErrors := BuildSDKTraceStatsPayload("", "", pcommon.NewMap(), metric)

			assert.Nil(t, payload)
			require.Len(t, conversionErrors, 1)
			assert.ErrorContains(t, conversionErrors[0].Err, test.wantErrMsg)
		})
	}
}

func TestSDKTraceStatsStatusCodeContract(t *testing.T) {
	for _, status := range []string{"STATUS_CODE_ERROR", "STATUS_CODE_OK", "STATUS_CODE_UNSET", ""} {
		t.Run(status, func(t *testing.T) {
			metric := sdkTraceMetric("s", 3, 1, map[string]string{"datadog.operation.name": "op"})
			attrs := metric.Histogram().DataPoints().At(0).Attributes()
			attrs.PutInt("http.response.status_code", 500)
			if status != "" {
				attrs.PutStr("status.code", status)
			}
			payload, errs := BuildSDKTraceStatsPayload("", "", pcommon.NewMap(), metric)
			require.Empty(t, errs)
			got := groupedByName(payload, "op")
			require.NotNil(t, got)
			if status == "STATUS_CODE_ERROR" {
				assert.Equal(t, uint64(3), got.Errors)
				assert.NotEmpty(t, got.ErrorSparseSketch)
				assert.Empty(t, got.OkSparseSketch)
			} else {
				assert.Zero(t, got.Errors)
				assert.NotEmpty(t, got.OkSparseSketch)
				assert.Empty(t, got.ErrorSparseSketch)
			}
		})
	}
}

func TestSDKTraceStatsRejectsInvalidDurations(t *testing.T) {
	for _, field := range []string{"sum", "min", "max"} {
		for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), -1} {
			t.Run(fmt.Sprintf("%s/%g", field, value), func(t *testing.T) {
				metric := sdkTraceMetric("s", 1, 1, nil)
				dp := metric.Histogram().DataPoints().At(0)
				switch field {
				case "sum":
					dp.SetSum(value)
				case "min":
					dp.SetMin(value)
				case "max":
					dp.SetMax(value)
				}
				payload, errs := BuildSDKTraceStatsPayload("", "", pcommon.NewMap(), metric)
				assert.Nil(t, payload)
				require.Len(t, errs, 1)
			})
		}
	}
}
