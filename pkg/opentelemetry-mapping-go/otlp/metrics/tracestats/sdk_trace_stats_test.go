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
	"math"
	"testing"
	"time"

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

func statsTags(tags []*stats.Tag) []string {
	values := make([]string, 0, len(tags))
	for _, tag := range tags {
		values = append(values, tag.Name+":"+tag.Value)
	}
	return values
}

func TestBuildSDKTraceStatsPayload(t *testing.T) {
	resourceAttributes := pcommon.NewMap()
	resourceAttributes.PutStr("service.name", "checkout-svc")
	resourceAttributes.PutStr("deployment.environment.name", "staging")
	resourceAttributes.PutStr("service.version", "1.2.3")
	resourceAttributes.PutStr("container.id", "container-123")
	resourceAttributes.PutStr("telemetry.sdk.language", "java")

	metric := sdkTraceMetric("s", 5, 2, map[string]string{
		"datadog.operation.name":   "http.request",
		"span.name":                "checkout",
		"datadog.span.type":        "web",
		"span.kind":                "SPAN_KIND_SERVER",
		"datadog.span.top_level":   "true",
		"status.code":              "STATUS_CODE_ERROR",
		"datadog.is_trace_root":    "true",
		"datadog.origin":           "synthetics",
		"rpc.response.status_code": "NOT_FOUND",
		"peer.service":             "users-db",
		"http.request.method":      "POST",
		"http.route":               "/users/:id",
	})
	datapoint := metric.Histogram().DataPoints().At(0)
	datapoint.Attributes().PutInt("http.response.status_code", 500)
	peerTags := datapoint.Attributes().PutEmptySlice("datadog.peer_tags")
	peerTags.AppendEmpty().SetStr("db.hostname:users-db")
	datapoint.SetStartTimestamp(pcommon.Timestamp(10))
	datapoint.SetTimestamp(pcommon.Timestamp(20))

	payload, conversionErrors := BuildSDKTraceStatsPayload("agent-host", "test-source", resourceAttributes, metric)

	require.Empty(t, conversionErrors)
	require.NotNil(t, payload)
	assert.Equal(t, "agent-host", payload.HostName)
	assert.Equal(t, "test-source", payload.Source)
	assert.Equal(t, "container-123", payload.ContainerId)
	assert.Equal(t, []string{"java"}, payload.Languages)
	assert.True(t, payload.Aggregate)
	require.Len(t, payload.Stats, 1)
	assert.Equal(t, int64(10), payload.Stats[0].Start)
	assert.Equal(t, int64(10), payload.Stats[0].Duration)

	groupedStats := groupedByName(payload, "http.request")
	require.NotNil(t, groupedStats)
	assert.Equal(t, "checkout-svc", groupedStats.Service)
	assert.Equal(t, "staging", groupedStats.Env)
	assert.Equal(t, "1.2.3", groupedStats.Version)
	assert.Equal(t, "checkout", groupedStats.Resource)
	assert.Equal(t, "server", groupedStats.SpanKind)
	assert.True(t, groupedStats.Synthetics)
	assert.Equal(t, uint64(5), groupedStats.Hits)
	assert.True(t, groupedStats.HasHits)
	assert.Equal(t, uint64(5), groupedStats.Errors)
	assert.True(t, groupedStats.HasErrors)
	assert.Equal(t, uint64(5), groupedStats.TopLevelHits)
	assert.Equal(t, uint64(2e9), groupedStats.Duration)
	assert.True(t, groupedStats.HasDuration)
	assert.Equal(t, int32(500), groupedStats.HttpStatusCode)
	assert.Equal(t, "5", groupedStats.GrpcStatusCode)
	assert.Equal(t, stats.Trilean_TRUE, groupedStats.IsTraceRoot)
	assert.Equal(t, []string{"db.hostname:users-db"}, groupedStats.PeerTags)
	assert.Equal(t, []string{"origin:synthetics", "peer.service:users-db", "span.type:web"}, statsTags(groupedStats.OtherTags))
	assert.Nil(t, groupedStats.OkSparseSketch)
	assert.Equal(t, int64(5), sparseSketch(t, groupedStats.ErrorSparseSketch).Basic.Count)

	raw, err := MarshalStatsPayload(payload)
	require.NoError(t, err)
	roundTrip := &stats.OTLPIntakeStatsPayload{}
	require.NoError(t, proto.Unmarshal(raw, roundTrip))
	assert.True(t, proto.Equal(payload, roundTrip))
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

func TestSDKIsErrorStatus(t *testing.T) {
	for _, status := range []string{"2", "ERROR", "Error", "error", "STATUS_CODE_ERROR", "status_code_error"} {
		t.Run(status, func(t *testing.T) {
			assert.True(t, sdkIsErrorStatus(status))
		})
	}
	for _, status := range []string{"", "0", "1", "OK", "STATUS_CODE_OK"} {
		t.Run(status, func(t *testing.T) {
			assert.False(t, sdkIsErrorStatus(status))
		})
	}
}

func TestBuildSDKTraceStatsPayloadPreservesExplicitHistogram(t *testing.T) {
	metric := sdkTraceMetric("ms", 6, 500, map[string]string{"datadog.operation.name": "op"})
	datapoint := metric.Histogram().DataPoints().At(0)
	datapoint.ExplicitBounds().FromRaw([]float64{10, 100})
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
	t.Run("cumulative histogram", func(t *testing.T) {
		metric := sdkTraceMetric("s", 1, 1, nil)
		metric.Histogram().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
		payload, conversionErrors := BuildSDKTraceStatsPayload("", "", pcommon.NewMap(), metric)
		assert.Nil(t, payload)
		assert.Empty(t, conversionErrors)
	})

	t.Run("unrelated metric", func(t *testing.T) {
		metric := sdkTraceMetric("s", 1, 1, nil)
		metric.SetName("custom.duration")
		payload, conversionErrors := BuildSDKTraceStatsPayload("", "", pcommon.NewMap(), metric)
		assert.Nil(t, payload)
		assert.Empty(t, conversionErrors)
	})

	t.Run("fallback bucket window", func(t *testing.T) {
		metric := sdkTraceMetric("s", 1, 1, map[string]string{"datadog.operation.name": "op"})
		metric.Histogram().DataPoints().At(0).SetTimestamp(pcommon.Timestamp(42))
		payload, conversionErrors := BuildSDKTraceStatsPayload("", "", pcommon.NewMap(), metric)
		require.Empty(t, conversionErrors)
		require.Len(t, payload.Stats, 1)
		assert.Equal(t, int64(42), payload.Stats[0].Start)
		assert.Equal(t, int64(10*time.Second), payload.Stats[0].Duration)
	})
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
