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
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unsafe"

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

// capturingMockSerializer extends MockSerializer to capture sketch data (including native histograms)
// passed through SendSketch.
type capturingMockSerializer struct {
	MockSerializer
	sketches []*metrics.SketchSeries
}

func (m *capturingMockSerializer) SendSketch(src metrics.SketchesSource) error {
	for src.MoveNext() {
		m.sketches = append(m.sketches, src.Current())
	}
	return nil
}

func TestSendHistograms(t *testing.T) {
	dp := pmetric.NewHistogramDataPoint()
	dp.ExplicitBounds().FromRaw([]float64{1, 5, 10})
	dp.BucketCounts().FromRaw([]uint64{1, 3, 5, 2})
	dp.SetCount(11)
	dp.SetSum(42.0)

	edp := pmetric.NewExponentialHistogramDataPoint()
	edp.SetScale(4)
	edp.SetZeroCount(5)
	edp.Positive().SetOffset(0)
	edp.Positive().BucketCounts().FromRaw([]uint64{10, 20, 30})
	edp.SetCount(65)
	edp.SetSum(100.0)

	sc := serializerConsumer{
		hosts:          make(map[string]struct{}),
		ecsFargateTags: make(map[string]struct{}),
		sketches: metrics.SketchSeriesList{
			{
				Name:     "test.histogram",
				Tags:     tagset.CompositeTagsFromSlice([]string{"env:test"}),
				Host:     "testhost",
				Interval: 10,
				Points: []metrics.SketchPoint{{
					Ts:     0,
					Sketch: &metrics.ExplicitBoundHistogramPoint{Point: dp},
				}},
				Source: metrics.MetricSourceOpenTelemetryCollectorUnknown,
			},
			{
				Name:     "test.exp.histogram",
				Tags:     tagset.CompositeTagsFromSlice([]string{"env:test"}),
				Host:     "testhost",
				Interval: 10,
				Points: []metrics.SketchPoint{{
					Ts:     0,
					Sketch: &metrics.ExponentialHistogramPoint{Point: edp},
				}},
				Source: metrics.MetricSourceOpenTelemetryCollectorUnknown,
			},
		},
	}

	mock := &capturingMockSerializer{}
	err := sc.Send(mock)
	require.NoError(t, err)

	var explicit, exponential []*metrics.SketchSeries
	for _, s := range mock.sketches {
		if len(s.Points) == 0 {
			continue
		}
		switch s.Points[0].Sketch.(type) {
		case metrics.ExplicitBoundProvider:
			explicit = append(explicit, s)
		case metrics.ExponentialProvider:
			exponential = append(exponential, s)
		}
	}

	require.Len(t, explicit, 1)
	assert.Equal(t, "test.histogram", explicit[0].Name)
	assert.Equal(t, "testhost", explicit[0].Host)
	assert.Equal(t, int64(10), explicit[0].Interval)
	require.Len(t, explicit[0].Points, 1)
	ep := explicit[0].Points[0].Sketch.(metrics.ExplicitBoundProvider)
	assert.Equal(t, uint64(11), ep.Count())
	assert.Equal(t, 42.0, ep.Sum())

	require.Len(t, exponential, 1)
	assert.Equal(t, "test.exp.histogram", exponential[0].Name)
	assert.Equal(t, "testhost", exponential[0].Host)
	require.Len(t, exponential[0].Points, 1)
	xp := exponential[0].Points[0].Sketch.(metrics.ExponentialProvider)
	assert.Equal(t, uint64(65), xp.Count())
	assert.Equal(t, int32(4), xp.Scale())
}

func TestSendHistograms_Empty(t *testing.T) {
	sc := serializerConsumer{
		hosts:          make(map[string]struct{}),
		ecsFargateTags: make(map[string]struct{}),
	}

	mock := &capturingMockSerializer{}
	err := sc.Send(mock)
	require.NoError(t, err)
	assert.Empty(t, mock.sketches)
}

func TestSendHistograms_Multiple(t *testing.T) {
	dp := pmetric.NewHistogramDataPoint()
	edp := pmetric.NewExponentialHistogramDataPoint()

	sc := serializerConsumer{
		hosts:          make(map[string]struct{}),
		ecsFargateTags: make(map[string]struct{}),
		sketches: metrics.SketchSeriesList{
			{Name: "hist1", Host: "host1", Points: []metrics.SketchPoint{{Sketch: &metrics.ExplicitBoundHistogramPoint{Point: dp}}}},
			{Name: "hist2", Host: "host2", Points: []metrics.SketchPoint{{Sketch: &metrics.ExplicitBoundHistogramPoint{Point: dp}}}},
			{Name: "hist3", Host: "host3", Points: []metrics.SketchPoint{{Sketch: &metrics.ExplicitBoundHistogramPoint{Point: dp}}}},
			{Name: "exp1", Host: "host1", Points: []metrics.SketchPoint{{Sketch: &metrics.ExponentialHistogramPoint{Point: edp}}}},
			{Name: "exp2", Host: "host2", Points: []metrics.SketchPoint{{Sketch: &metrics.ExponentialHistogramPoint{Point: edp}}}},
		},
	}

	mock := &capturingMockSerializer{}
	err := sc.Send(mock)
	require.NoError(t, err)

	var explicit, exponential []*metrics.SketchSeries
	for _, s := range mock.sketches {
		if len(s.Points) == 0 {
			continue
		}
		switch s.Points[0].Sketch.(type) {
		case metrics.ExplicitBoundProvider:
			explicit = append(explicit, s)
		case metrics.ExponentialProvider:
			exponential = append(exponential, s)
		}
	}

	require.Len(t, explicit, 3)
	assert.Equal(t, "hist1", explicit[0].Name)
	assert.Equal(t, "hist2", explicit[1].Name)
	assert.Equal(t, "hist3", explicit[2].Name)

	require.Len(t, exponential, 2)
	assert.Equal(t, "exp1", exponential[0].Name)
	assert.Equal(t, "exp2", exponential[1].Name)
}

// dimensionsMirror mirrors the memory layout of otlpmetrics.Dimensions.
// The field types and order must match exactly; see dimensions.go comment:
// "NOTE: Keep this in sync with the TestDimensions struct."
type dimensionsMirror struct {
	name                string
	tags                []string
	host                string
	originID            string
	originProduct       otlpmetrics.OriginProduct
	originSubProduct    otlpmetrics.OriginSubProduct
	originProductDetail otlpmetrics.OriginProductDetail
}

func makeTestDimensions(name, host string, tags []string) *otlpmetrics.Dimensions {
	m := dimensionsMirror{name: name, tags: tags, host: host}
	return (*otlpmetrics.Dimensions)(unsafe.Pointer(&m))
}

func TestConsumeExplicitBoundHistogram(t *testing.T) {
	dp := pmetric.NewHistogramDataPoint()
	dp.ExplicitBounds().FromRaw([]float64{1, 5, 10})
	dp.BucketCounts().FromRaw([]uint64{1, 3, 5, 2})
	dp.SetCount(11)
	dp.SetSum(42.0)

	dims := makeTestDimensions("http.request.duration", "web-1", []string{"env:prod", "service:api"})

	sc := serializerConsumer{
		extraTags: []string{"extra:tag"},
	}
	sc.ConsumeExplicitBoundHistogram(context.Background(), dims, 5_000_000_000, 10, dp, true)

	require.Len(t, sc.sketches, 1)
	s := sc.sketches[0]
	assert.Equal(t, "http.request.duration", s.Name)
	assert.Equal(t, "web-1", s.Host)
	assert.Equal(t, int64(10), s.Interval)
	assert.Equal(t, metrics.MetricSourceOpenTelemetryCollectorUnknown, s.Source)

	var tags []string
	s.Tags.ForEach(func(tag string) { tags = append(tags, tag) })
	assert.Contains(t, tags, "extra:tag")
	assert.Contains(t, tags, "env:prod")
	assert.Contains(t, tags, "service:api")

	require.Len(t, s.Points, 1)
	assert.Equal(t, int64(5), s.Points[0].Ts)

	ep, ok := s.Points[0].Sketch.(*metrics.ExplicitBoundHistogramPoint)
	require.True(t, ok)
	assert.Equal(t, uint64(11), ep.Count())
	assert.Equal(t, 42.0, ep.Sum())
	assert.Equal(t, []float64{1, 5, 10}, ep.ExplicitBounds())
	assert.Equal(t, []uint64{1, 3, 5, 2}, ep.BucketCounts())
}

func TestConsumeExponentialHistogram(t *testing.T) {
	dp := pmetric.NewExponentialHistogramDataPoint()
	dp.SetScale(4)
	dp.SetZeroCount(5)
	dp.Positive().SetOffset(0)
	dp.Positive().BucketCounts().FromRaw([]uint64{10, 20, 30})
	dp.Negative().SetOffset(1)
	dp.Negative().BucketCounts().FromRaw([]uint64{7, 8})
	dp.SetCount(80)
	dp.SetSum(200.0)

	dims := makeTestDimensions("http.request.latency", "api-2", []string{"region:us-east"})

	sc := serializerConsumer{
		extraTags: []string{"team:backend"},
	}
	sc.ConsumeExponentialHistogram(context.Background(), dims, 10_000_000_000, 30, dp)

	require.Len(t, sc.sketches, 1)
	s := sc.sketches[0]
	assert.Equal(t, "http.request.latency", s.Name)
	assert.Equal(t, "api-2", s.Host)
	assert.Equal(t, int64(30), s.Interval)
	assert.Equal(t, metrics.MetricSourceOpenTelemetryCollectorUnknown, s.Source)

	var tags []string
	s.Tags.ForEach(func(tag string) { tags = append(tags, tag) })
	assert.Contains(t, tags, "team:backend")
	assert.Contains(t, tags, "region:us-east")

	require.Len(t, s.Points, 1)
	assert.Equal(t, int64(10), s.Points[0].Ts)

	xp, ok := s.Points[0].Sketch.(*metrics.ExponentialHistogramPoint)
	require.True(t, ok)
	assert.Equal(t, int32(4), xp.Scale())
	assert.Equal(t, uint64(5), xp.ZeroCount())
	assert.Equal(t, uint64(80), xp.Count())
	assert.Equal(t, 200.0, xp.Sum())
	assert.Equal(t, []uint64{10, 20, 30}, xp.PositiveBucketCounts())
	assert.Equal(t, []uint64{7, 8}, xp.NegativeBucketCounts())
}

func TestConsumeHistograms_Accumulates(t *testing.T) {
	dp1 := pmetric.NewHistogramDataPoint()
	dp1.SetCount(5)
	dp2 := pmetric.NewExponentialHistogramDataPoint()
	dp2.SetCount(10)
	dp3 := pmetric.NewHistogramDataPoint()
	dp3.SetCount(15)

	dims := makeTestDimensions("metric", "host", nil)

	sc := serializerConsumer{}
	sc.ConsumeExplicitBoundHistogram(context.Background(), dims, 1_000_000_000, 10, dp1, false)
	sc.ConsumeExponentialHistogram(context.Background(), dims, 2_000_000_000, 10, dp2)
	sc.ConsumeExplicitBoundHistogram(context.Background(), dims, 3_000_000_000, 10, dp3, false)

	require.Len(t, sc.sketches, 3)
	assert.Equal(t, int64(1), sc.sketches[0].Points[0].Ts)
	assert.Equal(t, int64(2), sc.sketches[1].Points[0].Ts)
	assert.Equal(t, int64(3), sc.sketches[2].Points[0].Ts)
}
