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

	"go.opentelemetry.io/collector/pdata/pmetric"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
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
func (m *MockSerializer) SendServiceChecks(_ servicecheck.ServiceChecks) error { return nil }
func (m *MockSerializer) SendIterableSeries(_ metrics.SerieSource) error       { return nil }
func (m *MockSerializer) AreSeriesEnabled() bool                               { return true }
func (m *MockSerializer) SendSketch(_ metrics.SketchesSource) error            { return nil }
func (m *MockSerializer) AreSketchesEnabled() bool                             { return true }
func (m *MockSerializer) SendExplicitBucketHistograms(_ metrics.ExplicitBucketHistogramSource) error {
	return nil
}
func (m *MockSerializer) SendExponentialHistograms(_ metrics.ExponentialHistogramSource) error {
	return nil
}
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

// capturingMockSerializer extends MockSerializer to capture histogram data passed to Send methods.
type capturingMockSerializer struct {
	MockSerializer
	explicitHistograms    []*metrics.ExplicitBucketHistogramSeries
	exponentialHistograms []*metrics.ExponentialHistogramSeries
}

func (m *capturingMockSerializer) SendExplicitBucketHistograms(src metrics.ExplicitBucketHistogramSource) error {
	for src.MoveNext() {
		m.explicitHistograms = append(m.explicitHistograms, src.Current())
	}
	return nil
}

func (m *capturingMockSerializer) SendExponentialHistograms(src metrics.ExponentialHistogramSource) error {
	for src.MoveNext() {
		m.exponentialHistograms = append(m.exponentialHistograms, src.Current())
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
		explicitBucketHistograms: []*metrics.ExplicitBucketHistogramSeries{
			{
				Name:           "test.histogram",
				EnrichmentTags: tagset.CompositeTagsFromSlice([]string{"env:test"}),
				Host:           "testhost",
				Interval:       10,
				Points:         []pmetric.HistogramDataPoint{dp},
				Source:         metrics.MetricSourceOpenTelemetryCollectorUnknown,
			},
		},
		exponentialHistograms: []*metrics.ExponentialHistogramSeries{
			{
				Name:           "test.exp.histogram",
				EnrichmentTags: tagset.CompositeTagsFromSlice([]string{"env:test"}),
				Host:           "testhost",
				Interval:       10,
				Points:         []pmetric.ExponentialHistogramDataPoint{edp},
				Source:         metrics.MetricSourceOpenTelemetryCollectorUnknown,
			},
		},
	}

	mock := &capturingMockSerializer{}
	err := sc.Send(mock)
	require.NoError(t, err)

	require.Len(t, mock.explicitHistograms, 1)
	assert.Equal(t, "test.histogram", mock.explicitHistograms[0].Name)
	assert.Equal(t, "testhost", mock.explicitHistograms[0].Host)
	assert.Equal(t, int64(10), mock.explicitHistograms[0].Interval)
	require.Len(t, mock.explicitHistograms[0].Points, 1)
	assert.Equal(t, uint64(11), mock.explicitHistograms[0].Points[0].Count())
	assert.Equal(t, 42.0, mock.explicitHistograms[0].Points[0].Sum())

	require.Len(t, mock.exponentialHistograms, 1)
	assert.Equal(t, "test.exp.histogram", mock.exponentialHistograms[0].Name)
	assert.Equal(t, "testhost", mock.exponentialHistograms[0].Host)
	require.Len(t, mock.exponentialHistograms[0].Points, 1)
	assert.Equal(t, uint64(65), mock.exponentialHistograms[0].Points[0].Count())
	assert.Equal(t, int32(4), mock.exponentialHistograms[0].Points[0].Scale())
}

func TestSendHistograms_Empty(t *testing.T) {
	sc := serializerConsumer{
		hosts:          make(map[string]struct{}),
		ecsFargateTags: make(map[string]struct{}),
	}

	mock := &capturingMockSerializer{}
	err := sc.Send(mock)
	require.NoError(t, err)
	assert.Empty(t, mock.explicitHistograms)
	assert.Empty(t, mock.exponentialHistograms)
}

func TestSendHistograms_Multiple(t *testing.T) {
	sc := serializerConsumer{
		hosts:          make(map[string]struct{}),
		ecsFargateTags: make(map[string]struct{}),
		explicitBucketHistograms: []*metrics.ExplicitBucketHistogramSeries{
			{Name: "hist1", Host: "host1"},
			{Name: "hist2", Host: "host2"},
			{Name: "hist3", Host: "host3"},
		},
		exponentialHistograms: []*metrics.ExponentialHistogramSeries{
			{Name: "exp1", Host: "host1"},
			{Name: "exp2", Host: "host2"},
		},
	}

	mock := &capturingMockSerializer{}
	err := sc.Send(mock)
	require.NoError(t, err)

	require.Len(t, mock.explicitHistograms, 3)
	assert.Equal(t, "hist1", mock.explicitHistograms[0].Name)
	assert.Equal(t, "hist2", mock.explicitHistograms[1].Name)
	assert.Equal(t, "hist3", mock.explicitHistograms[2].Name)

	require.Len(t, mock.exponentialHistograms, 2)
	assert.Equal(t, "exp1", mock.exponentialHistograms[0].Name)
	assert.Equal(t, "exp2", mock.exponentialHistograms[1].Name)
}

func TestSliceExplicitBucketHistogramSource(t *testing.T) {
	data := []*metrics.ExplicitBucketHistogramSeries{
		{Name: "hist1"},
		{Name: "hist2"},
		{Name: "hist3"},
	}
	src := &sliceExplicitBucketHistogramSource{data: data, index: -1}

	assert.Equal(t, uint64(3), src.Count())
	assert.False(t, src.WaitForValue())

	assert.True(t, src.MoveNext())
	assert.Equal(t, "hist1", src.Current().Name)
	assert.True(t, src.MoveNext())
	assert.Equal(t, "hist2", src.Current().Name)
	assert.True(t, src.MoveNext())
	assert.Equal(t, "hist3", src.Current().Name)
	assert.False(t, src.MoveNext(), "should return false after last element")
}

func TestSliceExponentialHistogramSource(t *testing.T) {
	data := []*metrics.ExponentialHistogramSeries{
		{Name: "exp1"},
		{Name: "exp2"},
	}
	src := &sliceExponentialHistogramSource{data: data, index: -1}

	assert.Equal(t, uint64(2), src.Count())
	assert.False(t, src.WaitForValue())

	assert.True(t, src.MoveNext())
	assert.Equal(t, "exp1", src.Current().Name)
	assert.True(t, src.MoveNext())
	assert.Equal(t, "exp2", src.Current().Name)
	assert.False(t, src.MoveNext(), "should return false after last element")
}

func TestSliceExplicitBucketHistogramSource_Empty(t *testing.T) {
	src := &sliceExplicitBucketHistogramSource{data: nil, index: -1}
	assert.Equal(t, uint64(0), src.Count())
	assert.False(t, src.MoveNext())
}

func TestSliceExponentialHistogramSource_Empty(t *testing.T) {
	src := &sliceExponentialHistogramSource{data: nil, index: -1}
	assert.Equal(t, uint64(0), src.Count())
	assert.False(t, src.MoveNext())
}
