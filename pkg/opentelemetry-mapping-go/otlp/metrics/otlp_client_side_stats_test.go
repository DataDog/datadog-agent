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

package metrics

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
)

// sdkTraceMetric builds a single-datapoint delta histogram named like the DD-SDK trace metric.
func sdkTraceMetric(unit string, count uint64, sum float64, attrs map[string]string) pmetric.Metric {
	m := pmetric.NewMetric()
	m.SetName("traces.span.sdk.metrics.duration")
	m.SetUnit(unit)
	h := m.SetEmptyHistogram()
	h.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	dp := h.DataPoints().AppendEmpty()
	dp.SetCount(count)
	dp.SetSum(sum)
	dp.SetMin(0)
	dp.SetMax(sum)
	dp.BucketCounts().FromRaw([]uint64{count})
	for k, v := range attrs {
		dp.Attributes().PutStr(k, v)
	}
	return m
}

// remapSDK runs the SDK-trace remap and returns the emitted V4 stats payload, or nil.
func remapSDK(t testing.TB, m pmetric.Metric) *pb.OTLPIntakeStatsPayload {
	t.Helper()
	statsOut := make(chan []byte, 8)
	consumer := newTestConsumer()
	remapSDKTraceMetrics(zap.NewNop(), &consumer, statsOut, "", pcommon.NewMap(), m)
	close(statsOut)
	return drainSingleOTLPStats(t, statsOut)
}

func drainSingleOTLPStats(t testing.TB, statsOut <-chan []byte) *pb.OTLPIntakeStatsPayload {
	t.Helper()
	var payloads []*pb.OTLPIntakeStatsPayload
	for raw := range statsOut {
		var sp pb.OTLPIntakeStatsPayload
		require.NoError(t, proto.Unmarshal(raw, &sp))
		payloads = append(payloads, &sp)
	}
	if len(payloads) == 0 {
		return nil
	}
	require.Len(t, payloads, 1)
	return payloads[0]
}

// groupedByName returns the grouped-stats row whose operation Name matches, or nil.
func groupedByName(p *pb.OTLPIntakeStatsPayload, name string) *pb.StatsBucketV3_GroupedStats {
	if p == nil {
		return nil
	}
	for _, b := range p.Stats {
		for _, g := range b.Stats {
			if g.Name == name {
				return g
			}
		}
	}
	return nil
}

func sparseSketch(t testing.TB, data []byte) *pb.SparseSketch {
	t.Helper()
	require.NotEmpty(t, data)
	var sk pb.SparseSketch
	require.NoError(t, proto.Unmarshal(data, &sk))
	return &sk
}

func statsTags(tags []*pb.StatsTag) []string {
	values := make([]string, 0, len(tags))
	for _, tag := range tags {
		values = append(values, tag.Name+":"+tag.Value)
	}
	return values
}

// TestSDKTraceMetric_EmitsAPMStats drives one rich datapoint end-to-end and asserts the full
// mapping: grouped-stats fields, payload identity, no trace.* metric leak, non-billable host,
// and the V4 outer payload.
func TestSDKTraceMetric_EmitsAPMStats(t *testing.T) {
	statsOut := make(chan []byte, 8)
	translator := NewTestTranslator(t, WithRemapping(), WithOTLPStatsOut(statsOut))

	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	rattrs := rm.Resource().Attributes()
	for k, v := range map[string]string{
		"service.name": "checkout-svc", "deployment.environment.name": "staging",
		"service.version":    "1.2.3",
		"datadog.runtime_id": "abc-123", "host.name": "my-host",
	} {
		rattrs.PutStr(k, v)
	}
	rattrs.PutEmptySlice("datadog.process_tags").AppendEmpty().SetStr("entrypoint.name:server")
	m := sdkTraceMetric("s", 5, 2.0, map[string]string{
		"datadog.operation.name": "http.request", "span.name": "checkout",
		"datadog.span.type": "web", "span.kind": "SPAN_KIND_SERVER", "datadog.span.top_level": "true",
		"status.code": "STATUS_CODE_ERROR", "datadog.is_trace_root": "true",
		"datadog.origin": "synthetics", "http.request.method": "POST", "http.route": "/users/:id",
		"rpc.response.status_code": "NOT_FOUND", "peer.service": "users-db",
	})
	m.Histogram().DataPoints().At(0).Attributes().PutInt("http.response.status_code", 500)
	m.CopyTo(rm.ScopeMetrics().AppendEmpty().Metrics().AppendEmpty())

	consumer := newTestConsumer()
	_, err := translator.MapMetrics(context.Background(), md, &consumer, nil)
	require.NoError(t, err)
	close(statsOut)

	// trace.* must not leak onto the metric-series/sketch intake; an SDK-only payload is not billable.
	assert.Empty(t, consumer.data.Metrics.Sketches)
	for _, ts := range consumer.data.Metrics.TimeSeries {
		assert.NotContains(t, ts.Name, "trace.", "trace.* must not be emitted as a metric series")
	}
	assert.Empty(t, consumer.data.Hosts, "SDK-trace-only payload must not consume a billable host")

	// The payload uses the V4 wire contract consumed by the OTLP intake stats path.
	var sp pb.OTLPIntakeStatsPayload
	require.NoError(t, proto.Unmarshal(mustDrainRaw(t, statsOut), &sp))
	assert.Equal(t, "otlp-intake-metrics-sdk", sp.Source)
	assert.True(t, sp.Aggregate)
	require.Len(t, sp.Stats, 1)

	gs := groupedByName(&sp, "http.request")
	require.NotNil(t, gs)
	assert.Equal(t, "checkout-svc", gs.Service)
	assert.Equal(t, "staging", gs.Env)
	assert.Equal(t, "1.2.3", gs.Version)
	assert.Equal(t, "checkout", gs.Resource)
	assert.Equal(t, "Server", gs.SpanKind)
	assert.True(t, gs.Synthetics) // datadog.origin = synthetics
	assert.Equal(t, uint64(5), gs.Hits)
	assert.True(t, gs.HasHits)
	assert.Equal(t, uint64(5), gs.Errors) // status.code = STATUS_CODE_ERROR
	assert.True(t, gs.HasErrors)
	assert.Equal(t, uint64(5), gs.TopLevelHits) // datadog.span.top_level = true
	assert.Equal(t, uint64(2e9), gs.Duration)   // 2s scaled to nanoseconds
	assert.True(t, gs.HasDuration)
	assert.Equal(t, int32(500), gs.HttpStatusCode)
	assert.Equal(t, "5", gs.GrpcStatusCode) // NOT_FOUND
	assert.Equal(t, pb.OTLPTrilean_OTLP_TRUE, gs.IsTraceRoot)
	assert.Subset(t, statsTags(gs.OtherTags), []string{"origin:synthetics", "peer.service:users-db", "span.type:web"})
	assert.Empty(t, gs.OkSparseSketch)
	assert.Equal(t, int64(5), sparseSketch(t, gs.ErrorSparseSketch).Basic.Count)
}

func TestSDKTraceMetric_PreservesExplicitHistogram(t *testing.T) {
	m := sdkTraceMetric("ms", 6, 500, map[string]string{"datadog.operation.name": "op"})
	dp := m.Histogram().DataPoints().At(0)
	dp.ExplicitBounds().FromRaw([]float64{10, 100})
	dp.BucketCounts().FromRaw([]uint64{1, 2, 3})

	gs := groupedByName(remapSDK(t, m), "op")
	require.NotNil(t, gs)
	sketch := sparseSketch(t, gs.OkSparseSketch)
	assert.Equal(t, []int32{-32768, 0, 1, 2, 3, 4}, sketch.K)
	assert.Equal(t, []uint32{2, math.Float32bits(0.01), math.Float32bits(0.1), 1, 2, 3}, sketch.N)
	assert.Equal(t, int64(6), sketch.Basic.Count)
	assert.InDelta(t, 0.5, sketch.Basic.Sum, 1e-12)
}

// AgentHostname is set from the host argument on the outer StatsPayload.
func TestRemapSDKTraceMetric_AgentHostname(t *testing.T) {
	statsOut := make(chan []byte, 8)
	consumer := newTestConsumer()
	remapSDKTraceMetrics(zap.NewNop(), &consumer, statsOut, "host", pcommon.NewMap(),
		sdkTraceMetric("s", 1, 1.0, map[string]string{"datadog.operation.name": "op"}))
	close(statsOut)
	var sp pb.OTLPIntakeStatsPayload
	require.NoError(t, proto.Unmarshal(mustDrainRaw(t, statsOut), &sp))
	assert.Equal(t, "host", sp.HostName)
}

// A cumulative datapoint is dropped rather than mishandled as delta (no state to diff against).
func TestRemapSDKTraceMetric_CumulativeSkipped(t *testing.T) {
	m := sdkTraceMetric("s", 5, 2.0, map[string]string{"datadog.operation.name": "op"})
	m.Histogram().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	assert.Nil(t, remapSDK(t, m))
}

// The kill switch keeps the raw SDK histogram as a sketch instead of remapping to stats.
func TestSDKTraceMetric_OptOut(t *testing.T) {
	translator := NewTestTranslator(t, WithoutSDKTraceMetricsRemapping())
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "svc")
	rm.Resource().Attributes().PutStr("host.name", "my-host")
	sdkTraceMetric("s", 2, 1.0, map[string]string{
		"datadog.operation.name": "op", "span.name": "res",
	}).CopyTo(rm.ScopeMetrics().AppendEmpty().Metrics().AppendEmpty())

	consumer := newTestConsumer()
	_, err := translator.MapMetrics(context.Background(), md, &consumer, nil)
	require.NoError(t, err)
	require.Len(t, consumer.data.Metrics.Sketches, 1)
	assert.True(t, isSDKTraceMetric(consumer.data.Metrics.Sketches[0].Name))
	assert.Empty(t, consumer.data.Hosts, "SDK trace metric must never bill, even opted out")
}

type apmStatsTestConsumer struct {
	testConsumer
	payloads []*pb.OTLPIntakeStatsPayload
}

func (c *apmStatsTestConsumer) ConsumeOTLPStats(raw []byte) {
	var payload pb.OTLPIntakeStatsPayload
	if proto.Unmarshal(raw, &payload) == nil {
		c.payloads = append(c.payloads, &payload)
	}
}

// With no output channel, the payload is delivered through OTLPStatsConsumer.
func TestSDKTraceMetric_AgentConsumerFallback(t *testing.T) {
	translator := NewTestTranslator(t)
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "agent-svc")
	sdkTraceMetric("s", 2, 1.0, map[string]string{
		"datadog.operation.name": "op",
	}).CopyTo(rm.ScopeMetrics().AppendEmpty().Metrics().AppendEmpty())

	consumer := &apmStatsTestConsumer{testConsumer: newTestConsumer()}
	_, err := translator.MapMetrics(context.Background(), md, consumer, nil)
	require.NoError(t, err)
	require.Len(t, consumer.payloads, 1)
	gs := groupedByName(consumer.payloads[0], "op")
	require.NotNil(t, gs)
	assert.Equal(t, "agent-svc", gs.Service)
	assert.Equal(t, uint64(2), gs.Hits)
	assert.Empty(t, consumer.data.Metrics.Sketches)
	assert.Empty(t, consumer.data.Metrics.TimeSeries)
}

// mustDrainRaw returns the single raw StatsPayload bytes on the channel.
func mustDrainRaw(t testing.TB, statsOut <-chan []byte) []byte {
	t.Helper()
	raw, ok := <-statsOut
	require.True(t, ok, "expected a stats payload")
	return raw
}
