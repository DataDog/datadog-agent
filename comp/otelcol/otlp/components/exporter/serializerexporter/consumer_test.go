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

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/serializer/types"

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

func TestAddRunningMetric_Fargate(t *testing.T) {
	c := newTestSerializerConsumer(ddot, true)
	c.ConsumeHost("otel-host")

	c.addRunningMetric("agent-hostname", workloadIdentity{fargateTaskARN: "arn:aws:ecs:us-east-1:123:task/cluster/abc"})

	require.Len(t, c.series, 1)
	assert.Equal(t, "otel.ddot_collector.metrics.running.fargate", c.series[0].Name)
	assert.Empty(t, c.series[0].Host)
	assert.Contains(t, c.series[0].Tags.UnsafeToReadOnlySliceString(), "task_arn:arn:aws:ecs:us-east-1:123:task/cluster/abc")
}

func TestAddRunningMetric_AzureContainerApps(t *testing.T) {
	c := newTestSerializerConsumer(ddot, true)
	c.ConsumeHost("otel-host")

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

	c.addRunningMetric("agent-hostname", workloadIdentity{aca: &acaIdentity{name: "my-app"}})

	require.Len(t, c.series, 1)
	assert.Equal(t, "otel.ddot_collector.metrics.running", c.series[0].Name)
	assert.Equal(t, "agent-hostname", c.series[0].Host)
}
