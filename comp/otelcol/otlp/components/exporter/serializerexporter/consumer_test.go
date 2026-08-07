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
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/serializer/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConsumeOTLPStats(t *testing.T) {
	sc := serializerConsumer{extraTags: []string{"k:v"}, apmReceiverAddr: "http://localhost:1234/v0.6/stats"}
	sc.ConsumeOTLPStats([]byte{1, 2, 3})
	sc.ConsumeOTLPStats([]byte{4, 5, 6})
	require.Equal(t, [][]byte{{1, 2, 3}, {4, 5, 6}}, sc.otlpstats)
}

func TestSendOTLPStats(t *testing.T) {
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
			require.Equal(t, "application/x-protobuf", req.Header.Get("Content-Type"))
			require.Equal(t, "otlp", req.Header.Get("Dd-Protocol"))
			in, err := io.ReadAll(req.Body)
			defer req.Body.Close()
			require.NoError(t, err)
			assert.Equal(t, []byte{byte(called + 1)}, in)
			called++
		}))
		defer srv.Close()

		sc := serializerConsumer{extraTags: []string{"k:v"}, apmReceiverAddr: fmt.Sprintf("http://localhost:%s/v0.6/stats", port)}
		sc.ConsumeOTLPStats([]byte{1})
		sc.ConsumeOTLPStats([]byte{2})
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
		sc.ConsumeOTLPStats([]byte{1})
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
		sc.ConsumeOTLPStats([]byte{1})
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
