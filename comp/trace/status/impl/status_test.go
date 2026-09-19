// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package statusimpl

import (
	"bytes"
	"context"
	"encoding/json"
	"expvar"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/status"
	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

func TestStatusOut(t *testing.T) {
	traceStatusFixture(t)
	provides := NewComponent()

	headerProvider := provides.StatusProvider.Provider

	tests := []struct {
		name       string
		assertFunc func(t *testing.T)
	}{
		{"JSON", func(t *testing.T) {
			stats := make(map[string]interface{})
			require.NoError(t, headerProvider.JSON(false, stats))

			assert.NotEmpty(t, stats)
			apmStats := stats["apmStats"].(map[string]interface{})
			assert.Equal(t, float64(10), apmStats["uptime"])
		}},
		{"Text", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := headerProvider.Text(false, b)

			assert.NoError(t, err)

			assert.NotEmpty(t, b.String())
			assert.Contains(t, b.String(), "Hostname: trace-host")
		}},
		{"HTML", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := headerProvider.HTML(false, b)

			assert.NoError(t, err)

			assert.NotEmpty(t, b.String())
			assert.Contains(t, b.String(), "Hostname: trace-host")
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.assertFunc(t)
		})
	}
}

func TestSemanticCoreRendered(t *testing.T) {
	stats := map[string]interface{}{
		"apmStats": map[string]interface{}{
			"pid":                    "123",
			"uptime":                 10,
			"memstats":               map[string]interface{}{"Alloc": float64(1024)},
			"config":                 map[string]interface{}{"Hostname": "h", "ReceiverHost": "localhost", "ReceiverPort": float64(8126), "Endpoints": []interface{}{}},
			"receiver":               []interface{}{},
			"ratebyservice_filtered": map[string]interface{}{},
			"trace_writer":           map[string]interface{}{"Payloads": float64(0), "Traces": float64(0), "Events": float64(0), "Bytes": float64(0), "Errors": float64(0)},
			"stats_writer":           map[string]interface{}{"Payloads": float64(0), "StatsBuckets": float64(0), "Bytes": float64(0), "Errors": float64(0)},
			"trace_semantics": map[string]interface{}{
				"Source":      "remote-config",
				"ContentHash": "hash-rc",
				"Version":     "rc-1.0",
			},
		},
	}

	b := new(bytes.Buffer)
	require.NoError(t, status.RenderText(templatesFS, "traceagent.tmpl", b, stats))
	out := b.String()
	assert.Contains(t, out, "Trace Semantics")
	assert.Contains(t, out, "Source: Remote Config")
	assert.Contains(t, out, "hash-rc")
	assert.Contains(t, out, "rc-1.0")
}

var traceStatusFixtureOnce sync.Once

func traceStatusFixture(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	var fixture map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(`{
		"pid": "123",
		"uptime": 10,
		"config": {
			"Hostname": "trace-host",
			"ReceiverHost": "localhost",
			"ReceiverPort": 8126,
			"Endpoints": [{"Host": "https://trace.agent.example"}],
			"APIKey": "********"
		},
		"receiver": [],
		"ratebyservice_filtered": {"service:,env:": 0.5},
		"trace_writer": {"Payloads": 2, "Traces": 3, "Events": 4, "Bytes": 1024, "Errors": 1},
		"stats_writer": {"Payloads": 5, "StatsBuckets": 6, "Bytes": 2048, "Errors": 0},
		"trace_semantics": {"Source": "remote-config", "ContentHash": "hash-rc", "Version": "rc-1.0"}
	}`), &fixture))
	// These immutable fixtures are published once because expvar has no delete API.
	traceStatusFixtureOnce.Do(func() {
		t.Run("before_trace_init", func(t *testing.T) {
			require.Nil(t, expvar.Get("config"))
			provides := NewComponent()
			response, err := provides.Comp.GetStatusDetails(context.Background(), &pbcore.GetStatusDetailsRequest{})
			require.NoError(t, err)
			assert.JSONEq(t, `{"apmStats":{"error":"Trace Agent is not initialized yet"}}`, string(response.JsonPayload))
			require.Contains(t, response.NamedSections, "Details")
			text := response.NamedSections["Details"].Fields[""]
			assert.Contains(t, text, "Trace Agent is not initialized yet")
			assert.NotContains(t, text, "localhost:")
			var html bytes.Buffer
			require.NoError(t, provides.StatusProvider.Provider.HTML(false, &html))
			assert.Contains(t, html.String(), "Trace Agent is not initialized yet")
			assert.NotContains(t, html.String(), "localhost:")
		})
		for key, value := range fixture {
			require.Nil(t, expvar.Get(key), "unexpected expvar publisher for %s", key)
			expvar.Publish(key, expvar.Func(func() interface{} { return value }))
		}
	})
	return fixture
}

func TestGetStatusDetailsMatchesText(t *testing.T) {
	fixture := traceStatusFixture(t)
	snapshot, _ := expvar.Get(t.Name()).(*expvar.Map)
	if snapshot == nil {
		snapshot = expvar.NewMap(t.Name())
	}
	t.Cleanup(func() { snapshot.Init() })
	var samples atomic.Int32
	snapshot.Set("samples", expvar.Func(func() interface{} { return samples.Add(1) }))
	snapshot.Set("counter", expvar.Func(func() interface{} { return int64(9007199254740993) }))

	provides := NewComponent()
	response, err := provides.Comp.GetStatusDetails(context.Background(), &pbcore.GetStatusDetailsRequest{})
	require.NoError(t, err)
	require.Contains(t, response.NamedSections, "Details")
	assert.EqualValues(t, 1, samples.Load(), "rendered and JSON status must share one snapshot")
	var rawPayload struct {
		APMStats map[string]json.RawMessage `json:"apmStats"`
	}
	require.NoError(t, json.Unmarshal(response.JsonPayload, &rawPayload))
	for key, value := range fixture {
		assert.JSONEq(t, string(value), string(rawPayload.APMStats[key]), key)
	}
	assert.JSONEq(t, `{"samples":1,"counter":9007199254740993}`, string(rawPayload.APMStats[t.Name()]))
	assert.Contains(t, string(rawPayload.APMStats[t.Name()]), "9007199254740993")
	assert.Contains(t, rawPayload.APMStats, "memstats")

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(response.JsonPayload, &payload))
	var expected bytes.Buffer
	require.NoError(t, status.RenderText(templatesFS, "traceagent.tmpl", &expected, payload))
	assert.Equal(t, expected.String(), response.NamedSections["Details"].Fields[""])
	assert.Contains(t, expected.String(), "Hostname: trace-host")
	assert.Contains(t, expected.String(), "Default priority sampling rate: 50.0%")
	assert.Contains(t, expected.String(), "WARNING: Traces API errors (1 min): 1")
}
