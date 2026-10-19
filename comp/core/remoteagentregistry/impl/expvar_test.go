// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remoteagentregistryimpl

import (
	"context"
	"encoding/json"
	"expvar"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const dogstatsdTelemetry = `
# TYPE dogstatsd__udp_packets counter
dogstatsd__udp_packets{state="ok"} 10
dogstatsd__udp_packets{state="error"} 2
# TYPE dogstatsd__uds_packets counter
dogstatsd__uds_packets{transport="unix",state="ok"} 20
dogstatsd__uds_packets{transport="unixgram",state="ok"} 30
dogstatsd__uds_packets{transport="unix",state="error"} 4
dogstatsd__uds_packets{transport="tcp",state="ok"} 100
# TYPE dogstatsd__processed counter
dogstatsd__processed{message_type="metrics",state="ok",origin="host"} 40
dogstatsd__processed{message_type="metrics",state="error",origin="host"} 5
dogstatsd__processed{message_type="events",state="ok",origin="host"} 50
dogstatsd__processed{message_type="events",state="error",origin="host"} 6
dogstatsd__processed{message_type="service_checks",state="ok",origin="host"} 60
dogstatsd__processed{message_type="service_checks",state="error",origin="host"} 7
dogstatsd__processed{message_type="logs",state="ok",origin="host"} 100
`

func TestExpvarSeriesFromPromTextSelectsExpectedSeries(t *testing.T) {
	series := expvarSeriesFromPromText(dogstatsdTelemetry)

	values := make(map[string]int64)
	for _, value := range series {
		values[value.mappingID] += value.value
	}

	assert.Equal(t, int64(12), values["dogstatsd_udp_packets"])
	assert.Equal(t, int64(2), values["dogstatsd_udp_packet_reading_errors"])
	assert.Equal(t, int64(54), values["dogstatsd_uds_packets"])
	assert.Equal(t, int64(4), values["dogstatsd_uds_packet_reading_errors"])
	assert.Equal(t, int64(40), values["dogstatsd_metric_packets"])
	assert.Equal(t, int64(5), values["dogstatsd_metric_parse_errors"])
	assert.Equal(t, int64(50), values["dogstatsd_event_packets"])
	assert.Equal(t, int64(6), values["dogstatsd_event_parse_errors"])
	assert.Equal(t, int64(60), values["dogstatsd_service_check_packets"])
	assert.Equal(t, int64(7), values["dogstatsd_service_check_parse_errors"])
	assert.Len(t, values, len(dogstatsdExpvarMappings))
}

func TestExpvarSeriesFromPromTextRejectsMalformedAndNonCounterFamilies(t *testing.T) {
	assert.Empty(t, expvarSeriesFromPromText("# TYPE dogstatsd__udp_packets gauge\ndogstatsd__udp_packets 1\n"))
	assert.Empty(t, expvarSeriesFromPromText("# TYPE dogstatsd__udp_packets counter\ndogstatsd__udp_packets{state=\"ok\"} 1.5\n"))
	assert.Empty(t, expvarSeriesFromPromText("# TYPE dogstatsd__udp_packets counter\ndogstatsd__udp_packets{state=\"ok\"} 1"))
}

func TestRemoteExpvarValuesOnlyUseADP(t *testing.T) {
	provides, _, _, _, ipcComp := buildComponent(t)
	component := provides.Comp.(*remoteAgentRegistry)

	_ = buildAndRegisterRemoteAgent(t, ipcComp, component, adpAgentFlavor, "Agent Data Plane", "123",
		withTelemetryProvider(dogstatsdTelemetry),
	)
	_ = buildAndRegisterRemoteAgent(t, ipcComp, component, "process_agent", "Process Agent", "456",
		withTelemetryProvider(strings.ReplaceAll(dogstatsdTelemetry, "10", "1000")),
	)

	values := component.getRemoteExpvarValues()
	require.Contains(t, values, "agent-data-plane")
	assert.NotContains(t, values, "process-agent")
	assert.Equal(t, int64(12), values["agent-data-plane"]["dogstatsd_udp_packets"])
}

func TestRemoteExpvarCounterRemainsMonotonicAfterResetAndFailure(t *testing.T) {
	provides, _, cfg, _, ipcComp := buildComponent(t)
	cfg.SetInTest("remote_agent.registry.query_timeout", 100*time.Millisecond)
	component := provides.Comp.(*remoteAgentRegistry)

	agent := buildAndRegisterRemoteAgent(t, ipcComp, component, adpAgentFlavor, "Agent Data Plane", "123",
		withTelemetryProvider("# TYPE dogstatsd__udp_packets counter\ndogstatsd__udp_packets{state=\"ok\"} 10\n"),
	)

	values := component.getRemoteExpvarValues()
	assert.Equal(t, int64(10), values["agent-data-plane"]["dogstatsd_udp_packets"])

	// A second read inside the cache window reuses the same remote snapshot.
	agent.promText = "# TYPE dogstatsd__udp_packets counter\ndogstatsd__udp_packets{state=\"ok\"} 11\n"
	values = component.getRemoteExpvarValues()
	assert.Equal(t, int64(10), values["agent-data-plane"]["dogstatsd_udp_packets"])

	agent.promText = "# TYPE dogstatsd__udp_packets counter\ndogstatsd__udp_packets{state=\"ok\"} 3\n"
	component.expvarCache.values = nil
	values = component.getRemoteExpvarValues()
	assert.Equal(t, int64(13), values["agent-data-plane"]["dogstatsd_udp_packets"])

	agent.Stop()
	component.expvarCache.values = nil
	values = component.getRemoteExpvarValues()
	assert.Equal(t, int64(13), values["agent-data-plane"]["dogstatsd_udp_packets"])

	// A re-registered process gets a new identity, while the prior process's
	// accumulated value remains part of the monotonic remote total.
	_ = buildAndRegisterRemoteAgent(t, ipcComp, component, adpAgentFlavor, "Agent Data Plane", "456",
		withTelemetryProvider("# TYPE dogstatsd__udp_packets counter\ndogstatsd__udp_packets{state=\"ok\"} 4\n"),
	)
	component.expvarCache.values = nil
	values = component.getRemoteExpvarValues()
	assert.Equal(t, int64(17), values["agent-data-plane"]["dogstatsd_udp_packets"])
}

func TestRegisterExpvarsCombinesLocalValueAndPublishesRemoteValues(t *testing.T) {
	local := ensureExpvarInt(t, []string{"dogstatsd-udp", "Packets"})
	local.Set(8)

	provides, lc, _, _, ipcComp := buildComponent(t)
	component := provides.Comp.(*remoteAgentRegistry)
	require.NoError(t, lc.Start(context.Background()))
	t.Cleanup(func() { _ = lc.Stop(context.Background()) })

	_ = buildAndRegisterRemoteAgent(t, ipcComp, component, adpAgentFlavor, "Agent Data Plane", "123",
		withTelemetryProvider("# TYPE dogstatsd__udp_packets counter\ndogstatsd__udp_packets{state=\"ok\"} 5\n"),
	)
	component.expvarCache.values = nil

	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(expvar.Get("dogstatsd-udp").String()), &got))
	assert.Equal(t, float64(13), got["Packets"])

	var remote map[string]any
	require.NoError(t, json.Unmarshal([]byte(expvar.Get(remoteAgentsExpvarName).String()), &remote))
	assert.Equal(t, float64(5), remote["agent-data-plane"].(map[string]any)["dogstatsd-udp"].(map[string]any)["Packets"])
}

func ensureExpvarInt(t *testing.T, path []string) *expvar.Int {
	t.Helper()
	require.NotEmpty(t, path)

	root, ok := expvar.Get(path[0]).(*expvar.Map)
	if !ok {
		root = expvar.NewMap(path[0])
	}
	parent := root
	for _, key := range path[1 : len(path)-1] {
		child, ok := parent.Get(key).(*expvar.Map)
		if !ok {
			child = new(expvar.Map).Init()
			parent.Set(key, child)
		}
		parent = child
	}

	leaf := path[len(path)-1]
	if value, ok := parent.Get(leaf).(*expvar.Int); ok {
		return value
	}
	value := new(expvar.Int)
	parent.Set(leaf, value)
	return value
}
