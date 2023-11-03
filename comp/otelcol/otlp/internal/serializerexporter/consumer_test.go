// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serializerexporter

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
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
	sc := serializerConsumer{extraTags: []string{"k:v"}}
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
	withHandler := func(response http.Handler) *httptest.Server {
		srv := httptest.NewServer(response)
		_, port, err := net.SplitHostPort(srv.Listener.Addr().String())
		require.NoError(t, err)
		const cfgkey = "apm_config.receiver_port"
		config.Datadog.Set(cfgkey, port)
		defer func(old string) { config.Datadog.Set(cfgkey, old) }(config.Datadog.GetString(cfgkey))
		return srv
	}

	t.Run("ok", func(t *testing.T) {
		var called int
		srv := withHandler(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
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

		var sc serializerConsumer
		sc.ConsumeAPMStats(statsPayloads[0])
		sc.ConsumeAPMStats(statsPayloads[1])
		err := sc.Send(&MockSerializer{})
		require.NoError(t, err)
		require.Equal(t, called, 2)
	})

	t.Run("error", func(t *testing.T) {
		var called int
		srv := withHandler(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			io.Copy(io.Discard, req.Body)
			req.Body.Close()
			w.WriteHeader(http.StatusInternalServerError)
			called++
		}))
		defer srv.Close()

		var sc serializerConsumer
		sc.ConsumeAPMStats(statsPayloads[0])
		err := sc.Send(&MockSerializer{})
		require.ErrorContains(t, err, "HTTP Status code == 500 Internal Server Error")
		require.Equal(t, called, 1)
	})

	t.Run("error-msg", func(t *testing.T) {
		var called int
		srv := withHandler(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			io.Copy(io.Discard, req.Body)
			req.Body.Close()
			w.WriteHeader(http.StatusInternalServerError)
			w.Write(bytes.Repeat([]byte{'A'}, 2000))
			called++
		}))
		defer srv.Close()

		var sc serializerConsumer
		sc.ConsumeAPMStats(statsPayloads[0])
		err := sc.Send(&MockSerializer{})
		require.ErrorContains(t, err, "HTTP Status code == 500 Internal Server Error "+strings.Repeat("A", 1024))
		require.Equal(t, called, 1)
	})
}

// MockSerializer implements a no-op serializer.MetricSerializer.
type MockSerializer struct{}

func (m *MockSerializer) SendEvents(_ event.Events) error                         { return nil }
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
