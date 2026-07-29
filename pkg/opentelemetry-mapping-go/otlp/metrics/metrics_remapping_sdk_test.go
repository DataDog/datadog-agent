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
	"testing"

	"github.com/DataDog/sketches-go/ddsketch"
	sketchpb "github.com/DataDog/sketches-go/ddsketch/pb/sketchpb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
)

// sdkTraceMetric builds a delta histogram named like the DD-SDK trace metric with a
// single datapoint carrying the given attributes. It populates explicit buckets so
// the duration DDSketch can be constructed from the distribution.
func sdkTraceMetric(unit string, count uint64, sum float64, attrs map[string]string) pmetric.Metric {
	m := pmetric.NewMetric()
	m.SetName(sdkTraceMetricName)
	m.SetUnit(unit)
	h := m.SetEmptyHistogram()
	h.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	dp := h.DataPoints().AppendEmpty()
	dp.SetCount(count)
	dp.SetSum(sum)
	dp.SetMin(0)
	dp.SetMax(sum)
	// Single (-inf, +inf) bucket holding the whole population so the sketch has data.
	dp.BucketCounts().FromRaw([]uint64{count})
	for k, v := range attrs {
		dp.Attributes().PutStr(k, v)
	}
	return m
}

// remapSDK runs the SDK-trace remap and returns the single emitted client stats
// payload, or nil if none was produced. The remap publishes a proto-marshaled
// StatsPayload on the stats channel, so we drain and decode it.
func remapSDK(t testing.TB, m pmetric.Metric) *pb.ClientStatsPayload {
	t.Helper()
	statsOut := make(chan []byte, 8)
	baseDims := &Dimensions{name: m.Name()}
	remapSDKTraceMetrics(zap.NewNop(), statsOut, baseDims, pcommon.NewMap(), m)
	close(statsOut)
	return drainSingleClientStats(t, statsOut)
}

// drainSingleClientStats decodes every StatsPayload on the channel and returns the
// single client stats payload across them, or nil if there were none.
func drainSingleClientStats(t testing.TB, statsOut <-chan []byte) *pb.ClientStatsPayload {
	t.Helper()
	var payloads []*pb.ClientStatsPayload
	for raw := range statsOut {
		var sp pb.StatsPayload
		require.NoError(t, proto.Unmarshal(raw, &sp))
		payloads = append(payloads, sp.Stats...)
	}
	if len(payloads) == 0 {
		return nil
	}
	require.Len(t, payloads, 1)
	return payloads[0]
}

// groupedByName returns the grouped-stats row whose operation Name matches, or nil.
func groupedByName(p *pb.ClientStatsPayload, name string) *pb.ClientGroupedStats {
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

// sketchCount decodes a ddsketch-proto summary and returns its total count.
func sketchCount(t testing.TB, data []byte) float64 {
	t.Helper()
	require.NotEmpty(t, data, "summary must be a valid (possibly empty) sketch")
	var sk sketchpb.DDSketch
	require.NoError(t, proto.Unmarshal(data, &sk))
	decoded, err := ddsketch.FromProto(&sk)
	require.NoError(t, err)
	return decoded.GetCount()
}

func TestRemapSDKTraceMetric_DefaultMode(t *testing.T) {
	p := remapSDK(t, sdkTraceMetric("s", 5, 2.0, map[string]string{
		"datadog.operation.name": "http.request",
		"datadog.span.type":      "web",
		"datadog.span.top_level": "true",
		"datadog.origin":         "synthetics",
		"span.name":              "users.lookup",
		"span.kind":              "SERVER",
		"status.code":            "STATUS_CODE_ERROR",
	}))
	require.NotNil(t, p)

	gs := groupedByName(p, "http.request")
	require.NotNil(t, gs)
	assert.Equal(t, "users.lookup", gs.Resource)
	assert.Equal(t, "server", gs.SpanKind) // lowercased for Datadog APM
	assert.Equal(t, "web", gs.Type)
	assert.True(t, gs.Synthetics) // datadog.origin = synthetics
	assert.Equal(t, uint64(5), gs.Hits)
	assert.Equal(t, uint64(5), gs.Errors)       // status.code = STATUS_CODE_ERROR
	assert.Equal(t, uint64(5), gs.TopLevelHits) // datadog.span.top_level = true
	assert.Equal(t, uint64(2e9), gs.Duration)   // 2s scaled to nanoseconds
	assert.Zero(t, gs.HTTPStatusCode)           // non-HTTP span: left unset

	// Error datapoint: the population lands in ErrorSummary; OkSummary is the blank sketch.
	assert.Equal(t, float64(5), sketchCount(t, gs.ErrorSummary))
	assert.Zero(t, sketchCount(t, gs.OkSummary))
}

func TestRemapSDKTraceMetric_NotTopLevel(t *testing.T) {
	p := remapSDK(t, sdkTraceMetric("s", 3, 1.0, map[string]string{
		"datadog.operation.name": "op",
		"span.name":              "res",
	}))
	gs := groupedByName(p, "op")
	require.NotNil(t, gs)
	assert.Equal(t, uint64(3), gs.Hits)
	assert.Zero(t, gs.TopLevelHits)
	assert.Zero(t, gs.Errors)
	assert.False(t, gs.Synthetics)
	// Non-error datapoint: the population lands in OkSummary.
	assert.Equal(t, float64(3), sketchCount(t, gs.OkSummary))
	assert.Zero(t, sketchCount(t, gs.ErrorSummary))
}

func TestRemapSDKTraceMetric_ErrorGating(t *testing.T) {
	for _, tc := range []struct {
		name    string
		attrs   map[string]string
		wantErr bool
	}{
		{"status_code_full", map[string]string{"status.code": "STATUS_CODE_ERROR"}, true},
		{"status_code_short", map[string]string{"status.code": "ERROR"}, true},
		{"status_code_int", map[string]string{"status.code": "2"}, true},
		{"status_code_ok", map[string]string{"status.code": "OK"}, false},
		{"error_type_ignored", map[string]string{"error.type": "sql.timeout"}, false},
		{"no_status", map[string]string{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			attrs := map[string]string{"datadog.operation.name": "op"}
			for k, v := range tc.attrs {
				attrs[k] = v
			}
			gs := groupedByName(remapSDK(t, sdkTraceMetric("s", 4, 1.0, attrs)), "op")
			require.NotNil(t, gs)
			if tc.wantErr {
				assert.Equal(t, uint64(4), gs.Errors)
			} else {
				assert.Zero(t, gs.Errors)
			}
		})
	}
}

func TestRemapSDKTraceMetric_HTTPStatusSet(t *testing.T) {
	m := sdkTraceMetric("ms", 1, 500.0, map[string]string{
		"datadog.operation.name": "http.server.request",
		"span.kind":              "SERVER",
	})
	m.Histogram().DataPoints().At(0).Attributes().PutInt("http.response.status_code", 200)
	gs := groupedByName(remapSDK(t, m), "http.server.request")
	require.NotNil(t, gs)
	assert.Equal(t, uint32(200), gs.HTTPStatusCode)
}

func TestRemapSDKTraceMetric_OTelSemanticsFallback(t *testing.T) {
	// No datadog.operation.name: operation resolved from semconv.
	gs := groupedByName(remapSDK(t, sdkTraceMetric("s", 2, 1.0, map[string]string{
		"span.name":           "GET /users/:id",
		"span.kind":           "SERVER",
		"http.request.method": "GET",
	})), "http.server.request")
	require.NotNil(t, gs)
	assert.Equal(t, "GET /users/:id", gs.Resource)
}

// TestRemapSDKTraceMetric_CumulativeSkipped verifies a cumulative-temporality
// datapoint is dropped rather than mishandled as delta, since we have no state
// here to diff it against the prior cumulative value.
func TestRemapSDKTraceMetric_CumulativeSkipped(t *testing.T) {
	m := sdkTraceMetric("s", 5, 2.0, map[string]string{"datadog.operation.name": "op"})
	m.Histogram().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	assert.Nil(t, remapSDK(t, m))
}

// TestRemapSDKTraceMetric_SpanKindCasing verifies every span kind is lowercased.
func TestRemapSDKTraceMetric_SpanKindCasing(t *testing.T) {
	for in, want := range map[string]string{
		"SERVER":   "server",
		"CLIENT":   "client",
		"PRODUCER": "producer",
		"CONSUMER": "consumer",
		"INTERNAL": "internal",
		"":         "unspecified",
	} {
		gs := groupedByName(remapSDK(t, sdkTraceMetric("s", 1, 1.0, map[string]string{
			"datadog.operation.name": "op",
			"span.kind":              in,
		})), "op")
		require.NotNil(t, gs, "input %q", in)
		assert.Equal(t, want, gs.SpanKind, "input %q", in)
	}
}

// The SDK trace metric must never be prefixed by renameMetrics: it matches none
// of the host/kafka/internal rename rules.
func TestRenameMetrics_SDKTraceMetricUnchanged(t *testing.T) {
	m := sdkTraceMetric("s", 1, 1.0, nil)
	renameMetrics(m)
	assert.Equal(t, sdkTraceMetricName, m.Name())
}

// TestSDKTraceMetric_EmitsAPMStats verifies the end-to-end translator path: the SDK
// duration histogram becomes an APM stats payload (not trace.* series or sketches),
// with payload-level identity pulled from the resource attributes.
func TestSDKTraceMetric_EmitsAPMStats(t *testing.T) {
	statsOut := make(chan []byte, 8)
	translator := NewTestTranslator(t, WithRemapping(), WithStatsOut(statsOut))

	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	rattrs := rm.Resource().Attributes()
	rattrs.PutStr("service.name", "checkout-svc")
	rattrs.PutStr("deployment.environment.name", "staging")
	rattrs.PutStr("service.version", "1.2.3")
	sm := rm.ScopeMetrics().AppendEmpty()
	sdkTraceMetric("s", 3, 1.5, map[string]string{
		"datadog.operation.name": "http.request",
		"span.name":              "checkout",
	}).CopyTo(sm.Metrics().AppendEmpty())

	consumer := newTestConsumer()
	_, err := translator.MapMetrics(context.Background(), md, &consumer, nil)
	require.NoError(t, err)
	close(statsOut)

	// trace.* must not leak onto the metric-series/sketch intake.
	assert.Empty(t, consumer.data.Metrics.Sketches)
	for _, ts := range consumer.data.Metrics.TimeSeries {
		assert.NotContains(t, ts.Name, "trace.", "trace.* must not be emitted as a metric series")
	}

	p := drainSingleClientStats(t, statsOut)
	require.NotNil(t, p)
	assert.Equal(t, "checkout-svc", p.Service)
	assert.Equal(t, "staging", p.Env)
	assert.Equal(t, "1.2.3", p.Version)

	gs := groupedByName(p, "http.request")
	require.NotNil(t, gs)
	assert.Equal(t, "checkout-svc", gs.Service)
	assert.Equal(t, "checkout", gs.Resource)
	assert.Equal(t, uint64(3), gs.Hits)
	assert.Equal(t, uint64(1.5e9), gs.Duration) // 1.5s scaled to nanoseconds
	assert.Equal(t, float64(3), sketchCount(t, gs.OkSummary))
}

// TestSDKTraceMetric_DecoupledFromRemapping verifies the SDK trace metric is handled
// under WithSDKTraceMetrics alone, without WithRemapping — the case for Agent/DDOT
// paths that don't enable container/system remapping.
func TestSDKTraceMetric_DecoupledFromRemapping(t *testing.T) {
	statsOut := make(chan []byte, 8)
	translator := NewTestTranslator(t, WithSDKTraceMetrics(), WithStatsOut(statsOut))

	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "svc")
	sm := rm.ScopeMetrics().AppendEmpty()
	sdkTraceMetric("s", 2, 1.0, map[string]string{
		"datadog.operation.name": "op",
		"span.name":              "res",
	}).CopyTo(sm.Metrics().AppendEmpty())

	consumer := newTestConsumer()
	_, err := translator.MapMetrics(context.Background(), md, &consumer, nil)
	require.NoError(t, err)
	close(statsOut)

	gs := groupedByName(drainSingleClientStats(t, statsOut), "op")
	require.NotNil(t, gs)
	assert.Equal(t, uint64(2), gs.Hits)
	// The raw histogram must not also be emitted as a metric.
	assert.Empty(t, consumer.data.Metrics.Sketches)
	assert.Empty(t, consumer.data.Metrics.TimeSeries)
}

// TestSDKTraceMetric_NotBillableHost verifies that a payload containing only the
// SDK trace metric does not mark the host as billable (no ConsumeHost call).
func TestSDKTraceMetric_NotBillableHost(t *testing.T) {
	statsOut := make(chan []byte, 8)
	translator := NewTestTranslator(t, WithRemapping(), WithStatsOut(statsOut))

	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("host.name", "my-host")
	sm := rm.ScopeMetrics().AppendEmpty()
	sdkTraceMetric("s", 1, 1.0, map[string]string{
		"datadog.operation.name": "op",
		"span.name":              "res",
	}).CopyTo(sm.Metrics().AppendEmpty())

	consumer := newTestConsumer()
	_, err := translator.MapMetrics(context.Background(), md, &consumer, nil)
	require.NoError(t, err)

	assert.Empty(t, consumer.data.Hosts, "SDK-trace-only payload must not consume a billable host")
}
