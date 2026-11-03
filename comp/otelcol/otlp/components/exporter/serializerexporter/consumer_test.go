// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serializerexporter

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	otlpmetrics "github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/metrics"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/serializer/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tinylib/msgp/msgp"
)

var statsPayloads = []*pb.ClientStatsPayload{
	{
		Hostname:         "host",
		Env:              "prod",
		Version:          "v1.2",
		Lang:             "go",
		TracerVersion:    "v44",
		RuntimeID:        "123jkl",
		Sequence:         2,
		AgentAggregation: "blah",
		Service:          "mysql",
		ContainerID:      "abcdef123456",
		Tags:             []string{"a:b", "c:d"},
		Stats: []*pb.ClientStatsBucket{
			{
				Start:    10,
				Duration: 1,
				Stats: []*pb.ClientGroupedStats{
					{
						Service:        "kafka",
						Name:           "queue.add",
						Resource:       "append",
						HTTPStatusCode: 220,
						Type:           "queue",
						Hits:           15,
						Errors:         3,
						Duration:       143,
						OkSummary:      []byte{1, 2, 3},
						ErrorSummary:   []byte{4, 5, 6},
						TopLevelHits:   5,
					},
				},
			},
		},
	},
	{
		Hostname:         "host2",
		Env:              "prod2",
		Version:          "v1.22",
		Lang:             "go2",
		TracerVersion:    "v442",
		RuntimeID:        "123jkl2",
		Sequence:         22,
		AgentAggregation: "blah2",
		Service:          "mysql2",
		ContainerID:      "abcdef1234562",
		Tags:             []string{"a:b2", "c:d2"},
		Stats: []*pb.ClientStatsBucket{
			{
				Start:    102,
				Duration: 12,
				Stats: []*pb.ClientGroupedStats{
					{
						Service:        "kafka2",
						Name:           "queue.add2",
						Resource:       "append2",
						HTTPStatusCode: 2202,
						Type:           "queue2",
						Hits:           152,
						Errors:         32,
						Duration:       1432,
						OkSummary:      []byte{1, 2, 3},
						ErrorSummary:   []byte{4, 5, 6},
						TopLevelHits:   52,
					},
				},
			},
		},
	},
}

func TestConsumeAPMStats(t *testing.T) {
	sc := serializerConsumer{extraTags: []string{"k:v"}, apmReceiverAddr: "http://localhost:1234/v0.6/stats"}
	sc.ConsumeAPMStats(statsPayloads[0])
	require.Len(t, sc.apmstats, 1)
	sc.ConsumeAPMStats(statsPayloads[1])
	require.Len(t, sc.apmstats, 2)

	one := &pb.ClientStatsPayload{}
	two := &pb.ClientStatsPayload{}
	err := msgp.Decode(sc.apmstats[0], one)
	require.NoError(t, err)
	err = msgp.Decode(sc.apmstats[1], two)
	require.NoError(t, err)
	assert.Equal(t, one.String(), statsPayloads[0].String())
	assert.Equal(t, two.String(), statsPayloads[1].String())
}

func TestSendAPMStats(t *testing.T) {
	withHandler := func(response http.Handler) (*httptest.Server, string) {
		srv := httptest.NewServer(response)
		_, port, err := net.SplitHostPort(srv.Listener.Addr().String())
		require.NoError(t, err)
		return srv, port
	}

	t.Run("ok", func(t *testing.T) {
		var called int
		srv, port := withHandler(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
			require.Equal(t, req.URL.Path, "/v0.6/stats")
			in := &pb.ClientStatsPayload{}
			in.Reset()
			err := msgp.Decode(req.Body, in)
			defer req.Body.Close()
			require.NoError(t, err)
			// compare string representations of messages
			assert.Equal(t, statsPayloads[called].String(), in.String())
			called++
		}))
		defer srv.Close()

		sc := serializerConsumer{extraTags: []string{"k:v"}, apmReceiverAddr: fmt.Sprintf("http://localhost:%s/v0.6/stats", port)}
		sc.ConsumeAPMStats(statsPayloads[0])
		sc.ConsumeAPMStats(statsPayloads[1])
		err := sc.Send(&MockSerializer{})
		require.NoError(t, err)
		require.Equal(t, called, 2)
	})

	t.Run("error", func(t *testing.T) {
		var called int
		srv, port := withHandler(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			io.Copy(io.Discard, req.Body)
			req.Body.Close()
			w.WriteHeader(http.StatusInternalServerError)
			called++
		}))
		defer srv.Close()

		sc := serializerConsumer{extraTags: []string{"k:v"}, apmReceiverAddr: fmt.Sprintf("http://localhost:%s/v0.6/stats", port)}
		sc.ConsumeAPMStats(statsPayloads[0])
		err := sc.Send(&MockSerializer{})
		require.ErrorContains(t, err, "HTTP Status code == 500 Internal Server Error")
		require.Equal(t, called, 1)
	})

	t.Run("error-msg", func(t *testing.T) {
		var called int
		srv, port := withHandler(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			io.Copy(io.Discard, req.Body)
			req.Body.Close()
			w.WriteHeader(http.StatusInternalServerError)
			w.Write(bytes.Repeat([]byte{'A'}, 2000))
			called++
		}))
		defer srv.Close()

		sc := serializerConsumer{extraTags: []string{"k:v"}, apmReceiverAddr: fmt.Sprintf("http://localhost:%s/v0.6/stats", port)}
		sc.ConsumeAPMStats(statsPayloads[0])
		err := sc.Send(&MockSerializer{})
		require.ErrorContains(t, err, "HTTP Status code == 500 Internal Server Error "+strings.Repeat("A", 1024))
		require.Equal(t, called, 1)
	})
}

// MockSerializer implements a no-op serializer.MetricSerializer.
type MockSerializer struct{}

func (m *MockSerializer) SendEvents(_ event.Events) error { return nil }
func (m *MockSerializer) SendAgentShutdownEvent(_ context.Context, _ *event.Event) error {
	return nil
}
func (m *MockSerializer) SendServiceChecks(_ servicecheck.ServiceChecks) error    { return nil }
func (m *MockSerializer) SendIterableSeries(_ metrics.SerieSource) error          { return nil }
func (m *MockSerializer) AreSeriesEnabled() bool                                  { return true }
func (m *MockSerializer) SendSketch(_ metrics.SketchesSource) error               { return nil }
func (m *MockSerializer) AreSketchesEnabled() bool                                { return true }
func (m *MockSerializer) SendMetadata(_ marshaler.JSONMarshaler) error            { return nil }
func (m *MockSerializer) SendHostMetadata(_ marshaler.JSONMarshaler) error        { return nil }
func (m *MockSerializer) SendProcessesMetadata(_ interface{}) error               { return nil }
func (m *MockSerializer) SendAgentchecksMetadata(_ marshaler.JSONMarshaler) error { return nil }

func (m *MockSerializer) SendOrchestratorMetadata(_ []types.ProcessMessageBody, _, _ string, _ int) error {
	return nil
}

func (m *MockSerializer) SendOrchestratorManifests(_ []types.ProcessMessageBody, _, _ string) error {
	return nil
}

func newTestSerializerConsumer(ipath ingestionPath, standalone bool) *serializerConsumer {
	return &serializerConsumer{
		ipath:      ipath,
		hosts:      make(map[string]struct{}),
		standalone: standalone,
	}
}

func TestAddRunningMetric_NotDDOTPath(t *testing.T) {
	for _, ipath := range []ingestionPath{ossCollector, agentOTLPIngest} {
		c := newTestSerializerConsumer(ipath, true)
		c.ConsumeHost("otel-host")

		c.addRunningMetric("agent-hostname", workloadIdentity{})

		assert.Empty(t, c.series)
	}
}

func TestAddRunningMetric_NotStandalone(t *testing.T) {
	c := newTestSerializerConsumer(ddot, false)
	c.ConsumeHost("otel-host")

	c.addRunningMetric("agent-hostname", workloadIdentity{})

	assert.Empty(t, c.series)
}

func TestAddRunningMetric_HostOnly(t *testing.T) {
	c := newTestSerializerConsumer(ddot, true)
	c.ConsumeHost("otel-host")
	c.sawMetric = true

	c.addRunningMetric("agent-hostname", workloadIdentity{})

	require.Len(t, c.series, 1)
	assert.Equal(t, "otel.ddot_collector.metrics.running", c.series[0].Name)
	assert.Equal(t, "agent-hostname", c.series[0].Host)
}

func TestAddRunningMetric_NoSignals(t *testing.T) {
	c := newTestSerializerConsumer(ddot, true)

	c.addRunningMetric("agent-hostname", workloadIdentity{})

	assert.Empty(t, c.series)
}

// sawMetric is left false (the zero value) even though a host and a workload
// identity are both present, simulating a flush that only carried APM stats.
// The running metric must not fire purely because the environment identifies
// a workload; it must also have seen a real metric this flush.
func TestAddRunningMetric_NoRealMetricSeen(t *testing.T) {
	c := newTestSerializerConsumer(ddot, true)
	c.ConsumeHost("otel-host")

	c.addRunningMetric("agent-hostname", workloadIdentity{fargateTaskARN: "arn:aws:ecs:us-east-1:123:task/cluster/abc"})

	assert.Empty(t, c.series)
}

// No ConsumeHost here on purpose: a hostless workload reports a tag set rather
// than a host, so ConsumeHost is never called for it. Seeding a host would make
// this pass even if the emission were (incorrectly) gated on c.hosts.
func TestAddRunningMetric_Fargate(t *testing.T) {
	c := newTestSerializerConsumer(ddot, true)
	c.sawMetric = true

	c.addRunningMetric("agent-hostname", workloadIdentity{fargateTaskARN: "arn:aws:ecs:us-east-1:123:task/cluster/abc"})

	require.Len(t, c.series, 1)
	assert.Equal(t, "otel.ddot_collector.metrics.running.fargate", c.series[0].Name)
	assert.Empty(t, c.series[0].Host)
	assert.Contains(t, c.series[0].Tags.UnsafeToReadOnlySliceString(), "task_arn:arn:aws:ecs:us-east-1:123:task/cluster/abc")
}

// Hostless, for the same reason as TestAddRunningMetric_Fargate.
func TestAddRunningMetric_AzureContainerApps(t *testing.T) {
	c := newTestSerializerConsumer(ddot, true)
	c.sawMetric = true

	c.addRunningMetric("agent-hostname", workloadIdentity{aca: &acaIdentity{
		replica:        "replica-1",
		name:           "my-app",
		subscriptionID: "sub-123",
		resourceGroup:  "my-rg",
	}})

	require.Len(t, c.series, 1)
	assert.Equal(t, "otel.ddot_collector.metrics.running.azurecontainerapps", c.series[0].Name)
	assert.Empty(t, c.series[0].Host)
	tags := c.series[0].Tags.UnsafeToReadOnlySliceString()
	assert.Contains(t, tags, "name:my-app")
	assert.Contains(t, tags, "subscription_id:sub-123")
	assert.Contains(t, tags, "resource_group:my-rg")
	assert.Contains(t, tags, "replica:replica-1")
}

func TestAddRunningMetric_AzureContainerApps_IncompleteIdentityFallsBackToHost(t *testing.T) {
	c := newTestSerializerConsumer(ddot, true)
	c.ConsumeHost("otel-host")
	c.sawMetric = true

	c.addRunningMetric("agent-hostname", workloadIdentity{aca: &acaIdentity{name: "my-app"}})

	require.Len(t, c.series, 1)
	assert.Equal(t, "otel.ddot_collector.metrics.running", c.series[0].Name)
	assert.Equal(t, "agent-hostname", c.series[0].Host)
}

func TestAddRunningMetric_AzureContainerApps_IncompleteIdentityWithoutHostEmitsNothing(t *testing.T) {
	c := newTestSerializerConsumer(ddot, true)
	c.sawMetric = true

	c.addRunningMetric("agent-hostname", workloadIdentity{aca: &acaIdentity{name: "my-app"}})

	assert.Empty(t, c.series)
}

// Missing replica alone must also be treated as an incomplete identity (not
// just missing name/subscriptionID/resourceGroup), per
// https://datadoghq.atlassian.net/wiki/x/VglyrgE.
func TestAddRunningMetric_AzureContainerApps_MissingReplicaFallsBackToHost(t *testing.T) {
	c := newTestSerializerConsumer(ddot, true)
	c.ConsumeHost("otel-host")
	c.sawMetric = true

	c.addRunningMetric("agent-hostname", workloadIdentity{aca: &acaIdentity{
		name:           "my-app",
		subscriptionID: "sub-123",
		resourceGroup:  "my-rg",
	}})

	require.Len(t, c.series, 1)
	assert.Equal(t, "otel.ddot_collector.metrics.running", c.series[0].Name)
	assert.Equal(t, "agent-hostname", c.series[0].Host)
}

// histogramPointsWriter records what a distribution writes.
type histogramPointsWriter struct {
	explicit    []metrics.OtelExplicitHistogramPoints
	exponential []metrics.OtelExponentialHistogramPoints
	meta        []metrics.DistributionMetadata
}

func (w *histogramPointsWriter) WriteDDSketch(metrics.DistributionMetadata, int, metrics.DDSketchPoints) error {
	panic("unexpected DDSketch")
}

func (w *histogramPointsWriter) WriteOtelExplicitHistogram(meta metrics.DistributionMetadata, numPoints int, points metrics.OtelExplicitHistogramPoints) error {
	if numPoints != 1 {
		panic("expected one point")
	}
	w.meta = append(w.meta, meta)
	w.explicit = append(w.explicit, points)
	return nil
}

func (w *histogramPointsWriter) WriteOtelExponentialHistogram(meta metrics.DistributionMetadata, numPoints int, points metrics.OtelExponentialHistogramPoints) error {
	if numPoints != 1 {
		panic("expected one point")
	}
	w.meta = append(w.meta, meta)
	w.exponential = append(w.exponential, points)
	return nil
}

func TestConsumeExplicitBoundHistogram(t *testing.T) {
	pts := pmetric.NewHistogramDataPointSlice()

	p := pts.AppendEmpty()
	p.SetTimestamp(pcommon.Timestamp(42 * time.Second))
	p.Attributes().PutStr("k", "v")
	p.SetCount(6)
	p.SetSum(12.5)
	p.SetMin(0.5)
	p.SetMax(7)
	p.ExplicitBounds().FromRaw([]float64{1, 5})
	p.BucketCounts().FromRaw([]uint64{1, 2, 3})

	// No min, max or sum.
	p = pts.AppendEmpty()
	p.SetTimestamp(pcommon.Timestamp(43 * time.Second))
	p.SetCount(1)
	p.BucketCounts().FromRaw([]uint64{1})

	p = pts.AppendEmpty()
	p.SetFlags(pmetric.DefaultDataPointFlags.WithNoRecordedValue(true))

	p = pts.AppendEmpty()
	p.ExplicitBounds().FromRaw([]float64{2, 1})
	p.BucketCounts().FromRaw([]uint64{1, 2, 3})

	c := &serializerConsumer{extraTags: []string{"extra:tag"}}
	c.ConsumeExplicitBoundHistogram(context.Background(), &otlpmetrics.Dimensions{}, pts)
	require.Len(t, c.distributions, 2)
	assert.True(t, c.sawMetric)
	assert.Empty(t, c.sketches)

	w := &histogramPointsWriter{}
	for _, d := range c.distributions {
		require.NoError(t, d.WriteTo(w))
	}
	require.Len(t, w.explicit, 2)
	assert.ElementsMatch(t, []string{"extra:tag", "k:v"}, tagsOf(w.meta[0].Tags))
	assert.ElementsMatch(t, []string{"extra:tag"}, tagsOf(w.meta[1].Tags))
	assert.Equal(t, metrics.MetricSourceOpenTelemetryCollectorUnknown, w.meta[0].Source)

	ts, haveMin, haveMax, haveSum, minV, maxV, sum, cnt, bounds, counts := w.explicit[0].GetOtelExplicitHistogramPoint(0)
	assert.Equal(t, int64(42), ts)
	assert.True(t, haveMin)
	assert.True(t, haveMax)
	assert.True(t, haveSum)
	assert.Equal(t, 0.5, minV)
	assert.Equal(t, 7.0, maxV)
	assert.Equal(t, 12.5, sum)
	assert.Equal(t, uint64(6), cnt)
	assert.Equal(t, []float64{1, 5}, bounds)
	assert.Equal(t, []uint64{1, 2, 3}, counts)

	ts, haveMin, haveMax, haveSum, _, _, _, cnt, bounds, counts = w.explicit[1].GetOtelExplicitHistogramPoint(0)
	assert.Equal(t, int64(43), ts)
	assert.False(t, haveMin)
	assert.False(t, haveMax)
	assert.False(t, haveSum)
	assert.Equal(t, uint64(1), cnt)
	assert.Empty(t, bounds)
	assert.Equal(t, []uint64{1}, counts)
}

func TestConsumeExponentialHistogram(t *testing.T) {
	pts := pmetric.NewExponentialHistogramDataPointSlice()

	p := pts.AppendEmpty()
	p.SetTimestamp(pcommon.Timestamp(42 * time.Second))
	p.Attributes().PutInt("k", 1)
	p.SetCount(7)
	p.SetSum(-3)
	p.SetMin(-4)
	p.SetMax(2)
	p.SetScale(-2)
	p.SetZeroCount(1)
	p.SetZeroThreshold(1e-9)
	p.Positive().SetOffset(-1)
	p.Positive().BucketCounts().FromRaw([]uint64{1, 2})
	p.Negative().SetOffset(3)
	p.Negative().BucketCounts().FromRaw([]uint64{3})

	p = pts.AppendEmpty()
	p.SetFlags(pmetric.DefaultDataPointFlags.WithNoRecordedValue(true))

	p = pts.AppendEmpty()
	p.SetZeroThreshold(math.NaN())

	c := &serializerConsumer{}
	c.ConsumeExponentialHistogram(context.Background(), &otlpmetrics.Dimensions{}, pts)
	require.Len(t, c.distributions, 1)
	assert.True(t, c.sawMetric)

	w := &histogramPointsWriter{}
	require.NoError(t, c.distributions[0].WriteTo(w))
	require.Len(t, w.exponential, 1)
	assert.ElementsMatch(t, []string{"k:1"}, tagsOf(w.meta[0].Tags))

	ts, haveMin, haveMax, haveSum, minV, maxV, sum, zeroThr, cnt, zeroCnt, scale, posOffs, negOffs, posCnt, negCnt := w.exponential[0].GetOtelExponentialHistogramPoint(0)
	assert.Equal(t, int64(42), ts)
	assert.True(t, haveMin)
	assert.True(t, haveMax)
	assert.True(t, haveSum)
	assert.Equal(t, -4.0, minV)
	assert.Equal(t, 2.0, maxV)
	assert.Equal(t, -3.0, sum)
	assert.Equal(t, 1e-9, zeroThr)
	assert.Equal(t, uint64(7), cnt)
	assert.Equal(t, uint64(1), zeroCnt)
	assert.Equal(t, int32(-2), scale)
	assert.Equal(t, int32(-1), posOffs)
	assert.Equal(t, int32(3), negOffs)
	assert.Equal(t, []uint64{1, 2}, posCnt)
	assert.Equal(t, []uint64{3}, negCnt)
}

func TestConsumeOtelHistogramsEmptyBatch(t *testing.T) {
	c := &serializerConsumer{}
	c.ConsumeExplicitBoundHistogram(context.Background(), &otlpmetrics.Dimensions{}, pmetric.NewHistogramDataPointSlice())
	c.ConsumeExponentialHistogram(context.Background(), &otlpmetrics.Dimensions{}, pmetric.NewExponentialHistogramDataPointSlice())
	assert.Empty(t, c.distributions)
	assert.False(t, c.sawMetric)
}

func tagsOf(tags tagset.CompositeTags) []string {
	var out []string
	tags.ForEach(func(tag string) { out = append(out, tag) })
	return out
}

func TestInvalidExplicitHistogram(t *testing.T) {
	tests := []struct {
		name    string
		bounds  []float64
		counts  []uint64
		invalid bool
	}{
		{name: "no buckets", bounds: nil, counts: nil},
		{name: "single bucket without bounds", bounds: nil, counts: []uint64{5}},
		{name: "regular", bounds: []float64{-1, 0, 2.5}, counts: []uint64{1, 2, 3, 4}},
		{name: "bounds without counts", bounds: []float64{1}, counts: nil, invalid: true},
		{name: "too many counts", bounds: []float64{1}, counts: []uint64{1, 2, 3}, invalid: true},
		{name: "too few counts", bounds: []float64{1, 2}, counts: []uint64{1, 2}, invalid: true},
		{name: "unsorted", bounds: []float64{2, 1}, counts: []uint64{1, 2, 3}, invalid: true},
		{name: "duplicate", bounds: []float64{1, 1}, counts: []uint64{1, 2, 3}, invalid: true},
		{name: "NaN", bounds: []float64{math.NaN()}, counts: []uint64{1, 2}, invalid: true},
		{name: "+Inf", bounds: []float64{1, math.Inf(1)}, counts: []uint64{1, 2, 3}, invalid: true},
		{name: "-Inf", bounds: []float64{math.Inf(-1), 1}, counts: []uint64{1, 2, 3}, invalid: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.invalid, invalidExplicitHistogram(tt.bounds, tt.counts) != "")
		})
	}
}

func TestInvalidExponentialHistogram(t *testing.T) {
	tests := []struct {
		name    string
		set     func(p pmetric.ExponentialHistogramDataPoint)
		invalid bool
	}{
		{name: "empty", set: func(pmetric.ExponentialHistogramDataPoint) {}},
		{name: "regular", set: func(p pmetric.ExponentialHistogramDataPoint) {
			p.SetZeroThreshold(1e-9)
			p.Positive().SetOffset(-3)
			p.Positive().BucketCounts().FromRaw([]uint64{1, 2})
			p.Negative().SetOffset(5)
			p.Negative().BucketCounts().FromRaw([]uint64{3})
		}},
		{name: "last bucket at MaxInt32", set: func(p pmetric.ExponentialHistogramDataPoint) {
			p.Positive().SetOffset(math.MaxInt32 - 1)
			p.Positive().BucketCounts().FromRaw([]uint64{1, 2})
		}},
		{name: "empty buckets at MaxInt32 offset", set: func(p pmetric.ExponentialHistogramDataPoint) {
			p.Positive().SetOffset(math.MaxInt32)
		}},
		{name: "positive overflow", set: func(p pmetric.ExponentialHistogramDataPoint) {
			p.Positive().SetOffset(math.MaxInt32)
			p.Positive().BucketCounts().FromRaw([]uint64{1, 2})
		}, invalid: true},
		{name: "negative overflow", set: func(p pmetric.ExponentialHistogramDataPoint) {
			p.Negative().SetOffset(math.MaxInt32)
			p.Negative().BucketCounts().FromRaw([]uint64{1, 2})
		}, invalid: true},
		{name: "negative zero threshold", set: func(p pmetric.ExponentialHistogramDataPoint) {
			p.SetZeroThreshold(-1)
		}, invalid: true},
		{name: "NaN zero threshold", set: func(p pmetric.ExponentialHistogramDataPoint) {
			p.SetZeroThreshold(math.NaN())
		}, invalid: true},
		{name: "+Inf zero threshold", set: func(p pmetric.ExponentialHistogramDataPoint) {
			p.SetZeroThreshold(math.Inf(1))
		}, invalid: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := pmetric.NewExponentialHistogramDataPoint()
			tt.set(p)
			assert.Equal(t, tt.invalid, invalidExponentialHistogram(p) != "")
		})
	}
}

func TestOtelHistogramSeriesSinglePoint(t *testing.T) {
	explicit := &otelExplicitHistogramSeries{}
	exponential := &otelExponentialHistogramSeries{}
	assert.NotPanics(t, func() { explicit.GetOtelExplicitHistogramPoint(0) })
	assert.NotPanics(t, func() { exponential.GetOtelExponentialHistogramPoint(0) })
	for _, i := range []int{-1, 1} {
		assert.Panics(t, func() { explicit.GetOtelExplicitHistogramPoint(i) })
		assert.Panics(t, func() { exponential.GetOtelExponentialHistogramPoint(i) })
	}
}
