// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package expvarcompat

import (
	"encoding/json"
	"expvar"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeJSONPreservesLocalAndRemoteViews(t *testing.T) {
	body := []byte(`{
	  "dogstatsd-udp": {"Packets": 8, "Bytes": 42},
	  "aggregator": {"FlushCount": {"Series": {"LastFlush": 2, "Name": "Series"}}},
	  "unrelated": {"Gauge": 11}
	}`)
	remote := map[string]any{"agent-data-plane": map[string]any{
		"dogstatsd-udp": map[string]any{"Packets": int64(5), "PacketReadingErrors": int64(1)},
		"aggregator":    map[string]any{"FlushCount": map[string]any{"Series": map[string]any{"LastFlush": int64(3)}}},
		"forwarder":     map[string]any{"Transactions": map[string]any{"Success": int64(7)}},
	}}
	merged, err := MergeJSON(body, remote)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, decode(merged, &got))
	udp := got["dogstatsd-udp"].(map[string]any)
	assert.Equal(t, json.Number("13"), udp["Packets"])
	assert.Equal(t, json.Number("42"), udp["Bytes"])
	assert.Equal(t, json.Number("1"), udp["PacketReadingErrors"])
	series := got["aggregator"].(map[string]any)["FlushCount"].(map[string]any)["Series"].(map[string]any)
	assert.Equal(t, json.Number("5"), series["LastFlush"])
	assert.Equal(t, "Series", series["Name"])
	assert.Equal(t, json.Number("7"), got["forwarder"].(map[string]any)["Transactions"].(map[string]any)["Success"])
	assert.Equal(t, json.Number("11"), got["unrelated"].(map[string]any)["Gauge"])
	assert.NotContains(t, got, "remote_agents", "mapped telemetry must not be exposed as new expvars")
	assert.Contains(t, string(merged), `"Packets": 13`)
}

func TestMergeJSONSaturatesWithoutReplacingIncompatibleValues(t *testing.T) {
	body := []byte(`{"dogstatsd-udp":{"Packets":9223372036854775807,"Bad":"text"}}`)
	remote := map[string]any{"adp": map[string]any{"dogstatsd-udp": map[string]any{"Packets": int64(1), "Bad": int64(2)}}}
	merged, err := MergeJSON(body, remote)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, decode(merged, &got))
	udp := got["dogstatsd-udp"].(map[string]any)
	assert.Equal(t, json.Number("9223372036854775807"), udp["Packets"])
	assert.Equal(t, "text", udp["Bad"])
	assert.Equal(t, int64(math.MaxInt64), saturatingAdd(math.MaxInt64, 1))
}

func TestHandlerPassesBodyThroughWithoutRemoteValues(t *testing.T) {
	const body = "{\"unrelated\":{\"Gauge\":11}}"
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(body))
	})

	for name, source := range map[string]Source{
		"nil snapshot":   func() map[string]any { return nil },
		"empty snapshot": func() map[string]any { return map[string]any{} },
	} {
		t.Run(name, func(t *testing.T) {
			response := httptest.NewRecorder()
			Handler(next, source).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/debug/vars", nil))
			require.Equal(t, http.StatusOK, response.Code)
			assert.Equal(t, body, response.Body.String(), "the local body should not be decoded and re-encoded")
			assert.Equal(t, "application/json; charset=utf-8", response.Header().Get("Content-Type"))
		})
	}
}

func TestHandlerProjectsLateCoreLeafOnEveryRequest(t *testing.T) {
	const name = "remote-expvar-compat-late-core-test"
	source := Source(func() map[string]any {
		return map[string]any{"agent-data-plane": map[string]any{name: map[string]any{"Packets": int64(5)}}}
	})
	mux := http.NewServeMux()
	mux.Handle("/debug/vars", expvar.Handler())
	mux.HandleFunc("/other", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("other")) })
	handler := Handler(mux, source)

	read := func() map[string]any {
		t.Helper()
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/debug/vars", nil))
		require.Equal(t, http.StatusOK, response.Code)
		assert.Contains(t, response.Header().Get("Content-Type"), "application/json")
		var got map[string]any
		require.NoError(t, decode(response.Body.Bytes(), &got))
		return got
	}
	first := read()
	assert.NotContains(t, first, "remote_agents")
	assert.Equal(t, json.Number("5"), first[name].(map[string]any)["Packets"])
	before := ReadRoot(name, source)
	assert.Empty(t, before.Core)
	assert.Equal(t, float64(5), before.RemoteByAgent["agent-data-plane"]["Packets"])
	assert.Equal(t, float64(5), before.Combined["Packets"])

	root := expvar.NewMap(name)
	local := new(expvar.Int)
	local.Set(8)
	root.Set("Packets", local)
	assert.Equal(t, json.Number("13"), read()[name].(map[string]any)["Packets"])
	assert.Equal(t, "8", root.Get("Packets").String(), "the Core expvar stays separate")
	views := ReadRoot(name, source)
	assert.Equal(t, float64(8), views.Core["Packets"])
	assert.Equal(t, float64(5), views.RemoteByAgent["agent-data-plane"]["Packets"])
	assert.Equal(t, float64(13), views.Combined["Packets"])

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/other", nil))
	assert.Equal(t, "other", response.Body.String())
}
