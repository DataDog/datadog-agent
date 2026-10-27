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
	  "unrelated": {"Gauge": 11},
	  "remote_agents": {
	    "agent-data-plane": {
	      "dogstatsd-udp": {"Packets": 5, "PacketReadingErrors": 1},
	      "aggregator": {"FlushCount": {"Series": {"LastFlush": 3}}},
	      "forwarder": {"Transactions": {"Success": 7}}
	    }
	  }
	}`)
	merged, err := MergeJSON(body)
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
	assert.Equal(t, json.Number("5"), got["remote_agents"].(map[string]any)["agent-data-plane"].(map[string]any)["dogstatsd-udp"].(map[string]any)["Packets"])
	assert.Contains(t, string(merged), `"Packets": 13`)
}

func TestMergeJSONSaturatesWithoutReplacingIncompatibleValues(t *testing.T) {
	body := []byte(`{"dogstatsd-udp":{"Packets":9223372036854775807,"Bad":"text"},"remote_agents":{"adp":{"dogstatsd-udp":{"Packets":1,"Bad":2}}}}`)
	merged, err := MergeJSON(body)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, decode(merged, &got))
	udp := got["dogstatsd-udp"].(map[string]any)
	assert.Equal(t, json.Number("9223372036854775807"), udp["Packets"])
	assert.Equal(t, "text", udp["Bad"])
	assert.Equal(t, int64(math.MaxInt64), saturatingAdd(math.MaxInt64, 1))
}

func TestHandlerProjectsLateCoreLeafOnEveryRequest(t *testing.T) {
	const name = "remote-expvar-compat-late-core-test"
	expvar.Publish(remoteAgentsName, expvar.Func(func() any {
		return map[string]any{"agent-data-plane": map[string]any{name: map[string]any{"Packets": int64(5)}}}
	}))
	mux := http.NewServeMux()
	mux.Handle("/debug/vars", expvar.Handler())
	mux.HandleFunc("/other", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("other")) })
	handler := Handler(mux)

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
	assert.Equal(t, json.Number("5"), read()[name].(map[string]any)["Packets"])
	before := ReadRoot(name)
	assert.Empty(t, before.Core)
	assert.Equal(t, float64(5), before.RemoteByAgent["agent-data-plane"]["Packets"])
	assert.Equal(t, float64(5), before.Combined["Packets"])

	root := expvar.NewMap(name)
	local := new(expvar.Int)
	local.Set(8)
	root.Set("Packets", local)
	assert.Equal(t, json.Number("13"), read()[name].(map[string]any)["Packets"])
	assert.Equal(t, "8", root.Get("Packets").String(), "the Core expvar stays separate")
	views := ReadRoot(name)
	assert.Equal(t, float64(8), views.Core["Packets"])
	assert.Equal(t, float64(5), views.RemoteByAgent["agent-data-plane"]["Packets"])
	assert.Equal(t, float64(13), views.Combined["Packets"])

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/other", nil))
	assert.Equal(t, "other", response.Body.String())
}
