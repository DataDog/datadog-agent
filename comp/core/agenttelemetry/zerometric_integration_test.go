// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package agenttelemetry holds integration tests of the agent telemetry component: the real
// component, wired with fx, sends its payloads through its real sender to an in-process fakeintake.
// It lives outside the component's own modules (def, fx, impl) so that it can import fakeintake.
package agenttelemetry

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	agenttelemetry "github.com/DataDog/datadog-agent/comp/core/agenttelemetry/def"
	agenttelemetryfx "github.com/DataDog/datadog-agent/comp/core/agenttelemetry/fx"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"
	fakeintakeserver "github.com/DataDog/datadog-agent/test/fakeintake/server"
)

const (
	counterName = "zero_metric_it.counter"
	// controlName is a gauge with a constant non-zero value: zero_metric never excludes it, so it is
	// in every flush and proves that a flush arrived when the counter is absent from it.
	controlName = "zero_metric_it.control"
)

// metricValue is the part of a metric of the agent telemetry payload this test asserts on.
type metricValue struct {
	Value float64 `json:"value"`
}

// envelope is an agent telemetry payload: a single "agent-metrics" payload, or a "message-batch"
// whose payload is a list of payloads.
type envelope struct {
	RequestType string          `json:"request_type"`
	Payload     json.RawMessage `json:"payload"`
}

// flushes returns the metrics of every agent metrics payload fakeintake received, in arrival order.
// The runner never runs a schedule concurrently, so the arrival order is the order of the flushes.
func flushes(t require.TestingT, client *fakeintakeclient.Client) []map[string]metricValue {
	raw, err := client.GetRawPayloads("/api/v2/apmtelemetry")
	require.NoError(t, err)

	var out []map[string]metricValue
	var collect func(e envelope)
	collect = func(e envelope) {
		switch e.RequestType {
		case "agent-metrics":
			var p struct {
				Metrics map[string]metricValue `json:"metrics"`
			}
			require.NoError(t, json.Unmarshal(e.Payload, &p))
			out = append(out, p.Metrics)
		case "message-batch":
			var batch []envelope
			require.NoError(t, json.Unmarshal(e.Payload, &batch))
			for _, b := range batch {
				collect(b)
			}
		}
	}
	for _, p := range raw {
		data, err := aggregator.Inflate(p.Data, p.Encoding)
		require.NoError(t, err)
		var e envelope
		require.NoError(t, json.Unmarshal(data, &e), "payload: %s", data)
		collect(e)
	}
	return out
}

// TestZeroMetricExcludesIdleCounterFromSentPayloads checks that, with exclude.zero_metric, a counter
// is sent after it moves, is left out of the next flush when it doesn't move, and is sent again
// with the delta since it was last sent once it moves again, as the intake receives the payloads.
func TestZeroMetricExcludesIdleCounterFromSentPayloads(t *testing.T) {
	ready := make(chan bool, 1)
	fi := fakeintakeserver.NewServer(
		fakeintakeserver.WithAddress("127.0.0.1:0"),
		fakeintakeserver.WithReadyChannel(ready),
	)
	fi.Start()
	require.True(t, <-ready, "fakeintake server did not become ready")
	t.Cleanup(func() { _ = fi.Stop() })
	client := fakeintakeclient.NewClient(fi.URL())

	// Flush every second, starting one second after the component starts.
	datadogYAML := fmt.Sprintf(`
api_key: test-api-key
agent_telemetry:
  enabled: true
  logs_dd_url: %s
  logs_no_ssl: true
  errortracking:
    enabled: false
  profiles:
    - name: zero-metric-it
      metric:
        exclude:
          zero_metric: true
        metrics:
          - name: %s
          - name: %s
      schedule:
        start_after: 1
        iterations: 0
        period: 1
`, fi.URL(), counterName, controlName)

	deps := fxutil.Test[struct {
		fx.In
		Telemetry      telemetry.Component
		AgentTelemetry agenttelemetry.Component
	}](t,
		agenttelemetryfx.Module(),
		fx.Provide(func(t testing.TB) config.Component { return config.NewMockFromYAML(t, datadogYAML) }),
		fx.Provide(func(t testing.TB) log.Component { return logmock.New(t) }),
		// A fresh registry: the metrics are the test's input, and no other test's metrics leak in.
		telemetrymock.Module(),
	)
	require.NotNil(t, deps.AgentTelemetry)

	counter := deps.Telemetry.NewCounter("zero_metric_it", "counter", nil, "")
	deps.Telemetry.NewGauge("zero_metric_it", "control", nil, "").Set(1)

	const timeout, tick = 10 * time.Second, 100 * time.Millisecond

	// Flush A: the counter moved, it is sent with its delta.
	counter.Add(5)
	var flushA int
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		ff := flushes(c, client)
		flushA = -1
		for i, f := range ff {
			if _, ok := f[counterName]; ok {
				flushA = i
				break
			}
		}
		require.GreaterOrEqual(c, flushA, 0, "no flush with %s yet", counterName)
		assert.Equal(c, 5.0, ff[flushA][counterName].Value)
	}, timeout, tick)

	// Flush B, the next one: the counter didn't move, it is excluded. The control metric proves
	// that flush B arrived.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		ff := flushes(c, client)
		require.Greater(c, len(ff), flushA+1, "flush after the first %s not received yet", counterName)
		flushB := ff[flushA+1]
		assert.Contains(c, flushB, controlName)
		assert.NotContains(c, flushB, counterName, "a counter with a zero delta must be excluded by zero_metric")
	}, timeout, tick)

	// The counter moves again: the first flush sending it carries the delta since flush A. Every
	// flush in between arrived without it.
	counter.Add(2)
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		ff := flushes(c, client)
		flushC := -1
		for i := flushA + 1; i < len(ff); i++ {
			if _, ok := ff[i][counterName]; ok {
				flushC = i
				break
			}
			assert.Contains(c, ff[i], controlName)
		}
		require.Greater(c, flushC, flushA+1, "no flush with %s after it moved again yet", counterName)
		assert.Equal(c, 2.0, ff[flushC][counterName].Value)
	}, timeout, tick)
}
