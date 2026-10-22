// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
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

const compatibilityTelemetry = `
# TYPE dogstatsd__udp_packets counter
dogstatsd__udp_packets{state="ok"} 10
dogstatsd__udp_packets{state="error"} 2
# TYPE dogstatsd__uds_packets counter
dogstatsd__uds_packets{transport="unix",state="ok"} 20
dogstatsd__uds_packets{transport="unixgram",state="ok"} 30
dogstatsd__uds_packets{transport="unix",state="error"} 4
dogstatsd__uds_packets{transport="tcp",state="ok"} 100
# TYPE dogstatsd__uds_origin_detection_errors counter
dogstatsd__uds_origin_detection_errors 3
# TYPE dogstatsd__processed counter
dogstatsd__processed{message_type="metrics",state="ok",origin="host"} 40
dogstatsd__processed{message_type="metrics",state="error",origin="host"} 5
dogstatsd__processed{message_type="events",state="ok",origin="host"} 50
dogstatsd__processed{message_type="events",state="error",origin="host"} 6
dogstatsd__processed{message_type="service_checks",state="ok",origin="host"} 60
dogstatsd__processed{message_type="service_checks",state="error",origin="host"} 7
dogstatsd__processed{message_type="logs",state="ok",origin="host"} 100
# TYPE transactions__dropped counter
transactions__dropped{domain="metrics",endpoint="series"} 8
# TYPE transactions__retry_queue_size gauge
transactions__retry_queue_size{domain="metrics"} 4
# TYPE transactions__success counter
transactions__success{domain="metrics",endpoint="series",proto_version="v2"} 9
# TYPE transactions__errors counter
transactions__errors{domain="metrics",endpoint="series",error_type="dns_lookup_failure"} 1
transactions__errors{domain="metrics",endpoint="series",error_type="tls_handshake_failure"} 2
transactions__errors{domain="metrics",endpoint="series",error_type="connection_failure"} 3
transactions__errors{domain="metrics",endpoint="series",error_type="writing_failure"} 4
transactions__errors{domain="metrics",endpoint="series",error_type="invalid_request"} 5
transactions__errors{domain="metrics",endpoint="series",error_type="cant_send"} 6
# TYPE transactions__http_errors counter
transactions__http_errors{domain="metrics",endpoint="series",code="500"} 2
`

func TestExpvarSeriesFromPromTextSelectsExpectedSeries(t *testing.T) {
	series, parsed := expvarSeriesFromPromText(compatibilityTelemetry)
	require.True(t, parsed)

	values := make(map[string]int64)
	for _, value := range series {
		values[value.mappingID] += value.value
	}

	assert.Equal(t, int64(12), values["dogstatsd_udp_packets"])
	assert.Equal(t, int64(2), values["dogstatsd_udp_packet_reading_errors"])
	assert.Equal(t, int64(54), values["dogstatsd_uds_packets"])
	assert.Equal(t, int64(4), values["dogstatsd_uds_packet_reading_errors"])
	assert.Equal(t, int64(3), values["dogstatsd_uds_origin_detection_errors"])
	assert.Equal(t, int64(40), values["dogstatsd_metric_packets"])
	assert.Equal(t, int64(5), values["dogstatsd_metric_parse_errors"])
	assert.Equal(t, int64(50), values["dogstatsd_event_packets"])
	assert.Equal(t, int64(6), values["dogstatsd_event_parse_errors"])
	assert.Equal(t, int64(60), values["dogstatsd_service_check_packets"])
	assert.Equal(t, int64(7), values["dogstatsd_service_check_parse_errors"])
	assert.Equal(t, int64(8), values["forwarder_dropped_on_input"])
	assert.Equal(t, int64(4), values["forwarder_retry_queue_size"])
	assert.Equal(t, int64(9), values["forwarder_success"])
	assert.Equal(t, int64(21), values["forwarder_errors"])
	assert.Equal(t, int64(1), values["forwarder_dns_errors"])
	assert.Equal(t, int64(2), values["forwarder_tls_errors"])
	assert.Equal(t, int64(3), values["forwarder_connection_errors"])
	assert.Equal(t, int64(4), values["forwarder_wrote_request_errors"])
	assert.Equal(t, int64(5), values["forwarder_sent_request_errors"])
	assert.Equal(t, int64(2), values["forwarder_http_errors"])
	assert.Len(t, values, len(expvarMappings))
}

func TestExpvarSeriesFromPromTextRejectsTypeMismatchAndMalformedText(t *testing.T) {
	series, parsed := expvarSeriesFromPromText("# TYPE dogstatsd__udp_packets gauge\ndogstatsd__udp_packets 1\n")
	assert.True(t, parsed)
	assert.Empty(t, series)

	series, parsed = expvarSeriesFromPromText("# TYPE dogstatsd__udp_packets counter\ndogstatsd__udp_packets{state=\"ok\"} 1.5\n")
	assert.True(t, parsed)
	assert.Empty(t, series)

	series, parsed = expvarSeriesFromPromText("# TYPE dogstatsd__udp_packets counter\ndogstatsd__udp_packets{state=\"ok\"} 1")
	assert.False(t, parsed)
	assert.Empty(t, series)

}

func TestRemoteExpvarValuesOnlyUseADP(t *testing.T) {
	provides, _, _, _, ipcComp := buildComponent(t)
	component := provides.Comp.(*remoteAgentRegistry)

	_ = buildAndRegisterRemoteAgent(t, ipcComp, component, adpAgentFlavor, "Agent Data Plane", "123",
		withTelemetryProvider(compatibilityTelemetry),
	)
	_ = buildAndRegisterRemoteAgent(t, ipcComp, component, "process_agent", "Process Agent", "456",
		withTelemetryProvider(strings.ReplaceAll(compatibilityTelemetry, "10", "1000")),
	)

	values := component.getRemoteExpvarValues()
	require.Contains(t, values, "agent-data-plane")
	assert.NotContains(t, values, "process-agent")
	assert.Equal(t, int64(12), values["agent-data-plane"]["dogstatsd_udp_packets"])
}

func TestRemoteExpvarGaugeUsesLatestValueAndIsRemovedOnDeregistration(t *testing.T) {
	provides, _, _, _, ipcComp := buildComponent(t)
	component := provides.Comp.(*remoteAgentRegistry)
	agent := buildAndRegisterRemoteAgent(t, ipcComp, component, adpAgentFlavor, "Agent Data Plane", "123",
		withTelemetryProvider("# TYPE transactions__retry_queue_size gauge\ntransactions__retry_queue_size{domain=\"metrics\"} 4\n"),
	)

	values := component.getRemoteExpvarValues()
	assert.Equal(t, int64(4), values["agent-data-plane"]["forwarder_retry_queue_size"])
	assert.Empty(t, component.expvarCache.counters)
	assert.Len(t, component.expvarCache.gauges, 1)

	agent.promText = "# TYPE transactions__retry_queue_size gauge\ntransactions__retry_queue_size{domain=\"metrics\"} 2\n"
	component.expvarCache.values = nil
	values = component.getRemoteExpvarValues()
	assert.Equal(t, int64(2), values["agent-data-plane"]["forwarder_retry_queue_size"], "gauges should use the latest value, not accumulate")

	agent.Stop()
	component.agentMapMu.Lock()
	clear(component.agentMap)
	component.agentMapMu.Unlock()
	component.expvarCache.values = nil
	values = component.getRemoteExpvarValues()
	assert.NotContains(t, values, "agent-data-plane")
	assert.Empty(t, component.expvarCache.gauges)
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
	assert.Len(t, component.expvarCache.counters, 1)
	for _, state := range component.expvarCache.counters {
		assert.False(t, state.hasOrigin, "origin tracking is only used when the origin label is present")
	}

	// A second read inside the cache window reuses the same remote snapshot.
	agent.promText = "# TYPE dogstatsd__udp_packets counter\ndogstatsd__udp_packets{state=\"ok\"} 11\n"
	values = component.getRemoteExpvarValues()
	assert.Equal(t, int64(10), values["agent-data-plane"]["dogstatsd_udp_packets"])

	agent.promText = "# TYPE dogstatsd__udp_packets counter\ndogstatsd__udp_packets{state=\"ok\"} 3\n"
	component.expvarCache.values = nil
	values = component.getRemoteExpvarValues()
	assert.Equal(t, int64(13), values["agent-data-plane"]["dogstatsd_udp_packets"])

	agent.Stop()
	// Explicitly remove the disconnected fixture from the registry so this assertion exercises
	// counter retirement on deregistration rather than the registry's idle-timeout sweep.
	component.agentMapMu.Lock()
	clear(component.agentMap)
	component.agentMapMu.Unlock()
	component.expvarCache.values = nil
	values = component.getRemoteExpvarValues()
	assert.Equal(t, int64(13), values["agent-data-plane"]["dogstatsd_udp_packets"])
	assert.Empty(t, component.expvarCache.counters, "unregistered process series should be compacted")

	// A re-registered process gets a new identity, while the prior process's
	// accumulated value remains part of the monotonic remote total.
	_ = buildAndRegisterRemoteAgent(t, ipcComp, component, adpAgentFlavor, "Agent Data Plane", "456",
		withTelemetryProvider("# TYPE dogstatsd__udp_packets counter\ndogstatsd__udp_packets{state=\"ok\"} 4\n"),
	)
	component.expvarCache.values = nil
	values = component.getRemoteExpvarValues()
	assert.Equal(t, int64(17), values["agent-data-plane"]["dogstatsd_udp_packets"])
}

func TestRemoteExpvarOriginStateExpiresAfterIdle(t *testing.T) {
	provides, _, _, _, ipcComp := buildComponent(t)
	component := provides.Comp.(*remoteAgentRegistry)
	agent := buildAndRegisterRemoteAgent(t, ipcComp, component, adpAgentFlavor, "Agent Data Plane", "123",
		withTelemetryProvider("# TYPE dogstatsd__processed counter\ndogstatsd__processed{message_type=\"metrics\",state=\"ok\",origin=\"container_id://worker-1\"} 10\n"),
	)

	values := component.getRemoteExpvarValues()
	assert.Equal(t, int64(10), values["agent-data-plane"]["dogstatsd_metric_packets"])

	registeredAgents := component.GetRegisteredAgents()
	require.Len(t, registeredAgents, 1)
	series, parsed := expvarSeriesFromPromText(agent.promText)
	require.True(t, parsed)
	require.Len(t, series, 1)
	counterKey := remoteSeriesKey{
		agentIdentity: remoteAgentIdentity(registeredAgents[0]),
		agentName:     registeredAgents[0].SanitizedDisplayName,
		mappingID:     series[0].mappingID,
		labels:        series[0].labels,
	}
	state := component.expvarCache.counters[counterKey]
	require.NotNil(t, state)
	initialLastSeen := state.lastSeen

	// Presence refreshes last-seen even when the absolute counter does not change.
	state.lastSeen = time.Now().Add(-remoteExpvarOriginIdleTTL - time.Second)
	component.expvarCache.values = nil
	values = component.getRemoteExpvarValues()
	assert.Equal(t, int64(10), values["agent-data-plane"]["dogstatsd_metric_packets"])
	assert.True(t, state.lastSeen.After(initialLastSeen))
	assert.Contains(t, component.expvarCache.counters, counterKey)

	// Once the origin-labeled series disappears, its completed total is compacted and the
	// per-series state is evicted.
	agent.promText = ""
	state.lastSeen = time.Now().Add(-remoteExpvarOriginIdleTTL - time.Second)
	component.expvarCache.values = nil
	values = component.getRemoteExpvarValues()
	assert.Equal(t, int64(10), values["agent-data-plane"]["dogstatsd_metric_packets"])
	assert.NotContains(t, component.expvarCache.counters, counterKey)
	assert.Equal(t, int64(10), component.expvarCache.retiredTotals["agent-data-plane"]["dogstatsd_metric_packets"])
}

func TestWrapExpvarCreatesMissingCompatibilityPath(t *testing.T) {
	mapping := expvarMapping{
		id:              "test_new_compatibility_expvar",
		expvarPath:      []string{"remote-expvar-compat-test", "Transactions", "DroppedOnInput"},
		createIfMissing: true,
	}
	require.True(t, wrapExpvar(mapping))

	root, ok := expvar.Get("remote-expvar-compat-test").(*expvar.Map)
	require.True(t, ok)
	transactions, ok := root.Get("Transactions").(*expvar.Map)
	require.True(t, ok)
	assert.Equal(t, "0", transactions.Get("DroppedOnInput").String())
}

func TestExpvarMappingsResolve(t *testing.T) {
	for _, mapping := range expvarMappings {
		ensureExpvarInt(t, mapping.expvarPath)
		require.Truef(t, wrapExpvar(mapping), "mapping %q did not resolve", mapping.id)
	}
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
