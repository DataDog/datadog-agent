// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

// Package integrationtest checks that the health report forwarder's telemetry reaches the agent telemetry payload.
//
// It holds only this test on purpose: forwarder.errors is registered in the process-wide telemetry registry when
// pkg/util/telemetry is loaded, and other tests reset that registry (the telemetry mock, the telemetry fx module on
// stop), leaving the counter in a registry nothing gathers. A package of its own is a test binary of its own, where
// nothing resets the registry before this test runs.
package integrationtest

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	agenttelemetryfx "github.com/DataDog/datadog-agent/comp/core/agenttelemetry/fx"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	forwarderdef "github.com/DataDog/datadog-agent/comp/healthplatform/forwarder/def"
	forwarderfx "github.com/DataDog/datadog-agent/comp/healthplatform/forwarder/fx"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// agentTelemetryRoute is the agent endpoint returning the payload agent telemetry sends for its profiles
const agentTelemetryRoute = "/metadata/agent-telemetry"

// forwarderErrorsPipeline is the health report forwarder wired to agent telemetry through the process-wide
// telemetry registry, like in the Agent
type forwarderErrorsPipeline struct {
	forwarder forwarderdef.Component
	payload   http.HandlerFunc
}

// newForwarderErrorsPipeline builds the pipeline for a health intake at intakeURL. The forwarder records
// forwarder.errors in the registry agent telemetry gathers, and agent telemetry selects and tags it with the
// forwarder_stats profile of its embedded default profiles.
func newForwarderErrorsPipeline(t *testing.T, intakeURL string) *forwarderErrorsPipeline {
	t.Helper()
	deps := fxutil.Test[struct {
		fx.In
		Forwarder forwarderdef.Component
		Endpoints []api.EndpointProvider `group:"agent_endpoint"`
	}](t,
		forwarderfx.Module(),
		agenttelemetryfx.Module(),
		// forwarder.errors is registered in the global registry when pkg/util/telemetry is loaded: use the same
		// component as the Agent. The telemetry fx module would reset that registry when the test stops, leaving
		// the counter in a registry nothing gathers anymore.
		fx.Provide(func() telemetry.Component { return telemetryimpl.GetCompatComponent() }),
		// the configuration is not part of the behavior: only the intake URL and agent telemetry matter
		fx.Provide(func(t testing.TB) config.Component {
			return config.NewMockWithOverrides(t, map[string]interface{}{
				"api_key":                          "0000001",
				"health_platform.forwarder.dd_url": intakeURL,
				"agent_telemetry.enabled":          true,
			})
		}),
		fx.Provide(func(t testing.TB) log.Component { return logmock.New(t) }),
	)

	for _, endpoint := range deps.Endpoints {
		if endpoint.Route() == agentTelemetryRoute {
			return &forwarderErrorsPipeline{forwarder: deps.Forwarder, payload: endpoint.HandlerFunc()}
		}
	}
	require.FailNow(t, "agent telemetry provides no "+agentTelemetryRoute+" endpoint")
	return nil
}

// forwarderErrorsSeries returns every forwarder.errors series of the next agent telemetry payload. Agent
// telemetry reports counters as the increase since its previous payload, so a first call sets the baseline.
func (p *forwarderErrorsPipeline) forwarderErrorsSeries(t *testing.T) []metricSeries {
	t.Helper()
	recorder := httptest.NewRecorder()
	p.payload(recorder, httptest.NewRequest(http.MethodGet, agentTelemetryRoute, nil))
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var payload any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	return findMetricSeries(payload, "forwarder.errors")
}

// metricSeries is one series of an agent telemetry metric
type metricSeries struct {
	Value float64        `json:"value"`
	Tags  map[string]any `json:"tags"`
}

// findMetricSeries returns the series named name in every metrics map of an agent telemetry payload, whatever
// the batching of the payload
func findMetricSeries(node any, name string) []metricSeries {
	var series []metricSeries
	switch value := node.(type) {
	case map[string]any:
		if metrics, ok := value["metrics"].(map[string]any); ok {
			series = append(series, decodeSeries(metrics[name])...)
		}
		for key, child := range value {
			if key != "metrics" {
				series = append(series, findMetricSeries(child, name)...)
			}
		}
	case []any:
		for _, child := range value {
			series = append(series, findMetricSeries(child, name)...)
		}
	}
	return series
}

// decodeSeries decodes a metric of a metrics map, given as one series or a list of series
func decodeSeries(raw any) []metricSeries {
	if raw == nil {
		return nil
	}
	encoded, _ := json.Marshal(raw)
	var list []metricSeries
	if json.Unmarshal(encoded, &list) == nil {
		return list
	}
	var single metricSeries
	if json.Unmarshal(encoded, &single) == nil {
		return []metricSeries{single}
	}
	return nil
}

// refusedURL returns the URL of a local port nothing listens on
func refusedURL(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	url := "http://" + listener.Addr().String()
	require.NoError(t, listener.Close())
	return url
}

// TestForwarderErrorsReachAgentTelemetry checks that the requests to the health intake that get no HTTP
// response are reported by agent telemetry in forwarder.errors, with the tags of the forwarder_stats profile.
//
// Don't add a test resetting the telemetry registry to this package: see the package documentation.
func TestForwarderErrorsReachAgentTelemetry(t *testing.T) {
	report := &healthplatformpayload.HealthReport{}

	t.Run("connection refused", func(t *testing.T) {
		pipeline := newForwarderErrorsPipeline(t, refusedURL(t))
		pipeline.forwarderErrorsSeries(t)

		_, err := pipeline.forwarder.Send(context.Background(), report)
		require.Error(t, err)

		assert.Equal(t, []metricSeries{{Value: 1, Tags: map[string]any{
			"emitter":    "agent", // added by agent telemetry to every metric
			"error_type": "connection_failure",
			"endpoint":   "agenthealth",
			"forwarder":  "health_platform",
		}}}, pipeline.forwarderErrorsSeries(t), "the domain tag must be dropped by the profile")
	})

	t.Run("timeout", func(t *testing.T) {
		unblock := make(chan struct{})
		intake := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			select {
			case <-unblock:
			case <-r.Context().Done():
			}
		}))
		t.Cleanup(intake.Close)
		t.Cleanup(func() { close(unblock) })
		pipeline := newForwarderErrorsPipeline(t, intake.URL)
		pipeline.forwarderErrorsSeries(t)

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		_, err := pipeline.forwarder.Send(ctx, report)
		require.Error(t, err)

		assert.Equal(t, []metricSeries{{Value: 1, Tags: map[string]any{
			"emitter":    "agent", // added by agent telemetry to every metric
			"error_type": "timeout",
			"endpoint":   "agenthealth",
			"forwarder":  "health_platform",
		}}}, pipeline.forwarderErrorsSeries(t))
	})

	t.Run("canceled by the caller", func(t *testing.T) {
		pipeline := newForwarderErrorsPipeline(t, refusedURL(t))
		pipeline.forwarderErrorsSeries(t)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := pipeline.forwarder.Send(ctx, report)
		require.Error(t, err)

		assert.Empty(t, pipeline.forwarderErrorsSeries(t), "a request the caller canceled must not be counted")
	})

	t.Run("intake answers", func(t *testing.T) {
		intake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusAccepted)
		}))
		t.Cleanup(intake.Close)
		pipeline := newForwarderErrorsPipeline(t, intake.URL)
		pipeline.forwarderErrorsSeries(t)

		_, err := pipeline.forwarder.Send(context.Background(), report)
		require.NoError(t, err)

		assert.Empty(t, pipeline.forwarderErrorsSeries(t), "no forwarder.errors without a transport error")
	})
}
