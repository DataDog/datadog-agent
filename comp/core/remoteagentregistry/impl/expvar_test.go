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
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	demultiplexerimpl "github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/impl"
	configcomponent "github.com/DataDog/datadog-agent/comp/core/config"
	hostnameimpl "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	logcomponent "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	pidmapfx "github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap/fx"
	replaymock "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/fx-mock"
	dogstatsdserver "github.com/DataDog/datadog-agent/comp/dogstatsd/server/def"
	dogstatsdserverimpl "github.com/DataDog/datadog-agent/comp/dogstatsd/server/impl"
	serverdebugmock "github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug/mock"
	filterlistmock "github.com/DataDog/datadog-agent/comp/filterlist/fx-mock"
	offlinereporter "github.com/DataDog/datadog-agent/comp/offlinereporter/def"
	offlinereportermock "github.com/DataDog/datadog-agent/comp/offlinereporter/mock"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	metricscompression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	_ "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/impl"
)

var agentExpvarSetup sync.Once

func initializeAgentExpvars(t *testing.T) {
	agentExpvarSetup.Do(func() {
		_ = fxutil.Test[dogstatsdserver.Component](t, fx.Options(
			fx.Provide(func() logcomponent.Component { return logmock.New(t) }),
			fx.Provide(func() configcomponent.Component {
				return configcomponent.NewMockWithOverrides(t, map[string]interface{}{"dogstatsd_port": 0})
			}),
			telemetrymock.Module(),
			hostnameimpl.MockModule(),
			serverdebugmock.MockModule(),
			replaymock.MockModule(),
			pidmapfx.Module(),
			demultiplexerimpl.FakeSamplerMockModule(),
			workloadmetafxmock.MockModule(workloadmeta.NewParams()),
			logscompression.MockModule(),
			metricscompression.MockModule(),
			filterlistmock.MockModule(),
			fx.Provide(func() offlinereporter.Component { return offlinereportermock.Mock(t) }),
			fxutil.ProvideComponentConstructor(dogstatsdserverimpl.NewComponent),
			fx.Supply(dogstatsdserver.Params{Serverless: false}),
		))
	})
}

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
# TYPE transactions__retry_queue_size gauge
transactions__retry_queue_size{domain="metrics"} 4
# TYPE transactions__success counter
transactions__success{domain="metrics",endpoint="/api/v1/series",proto_version="HTTP/2.0"} 9
transactions__success{domain="metrics",endpoint="/api/v1/validate",proto_version="HTTP/2.0"} 1000
# TYPE transactions__errors counter
transactions__errors{domain="metrics",endpoint="/api/v1/series",error_type="dns_error"} 1
transactions__errors{domain="metrics",endpoint="/api/v1/series",error_type="tls_error"} 2
transactions__errors{domain="metrics",endpoint="/api/v1/series",error_type="connection_error"} 3
transactions__errors{domain="metrics",endpoint="/api/v1/series",error_type="wrote_request_error"} 4
transactions__errors{domain="metrics",endpoint="/api/v1/series",error_type="sent_request_error"} 5
transactions__errors{domain="metrics",endpoint="/api/v1/series",error_type="cant_send"} 6
transactions__errors{domain="metrics",endpoint="/api/v1/validate",error_type="dns_error"} 1000
# TYPE transactions__http_errors counter
transactions__http_errors{domain="metrics",endpoint="/api/v1/series",code="500"} 2
transactions__http_errors{domain="metrics",endpoint="/api/v1/validate",code="500"} 1000
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

func TestExpvarSeriesDistinguishesNoMatchFromMatchedZero(t *testing.T) {
	noMatch, parsed := expvarSeriesFromPromText(`# TYPE dogstatsd__processed counter
dogstatsd__processed{message_type="logs",state="ok"} 0
`)
	require.True(t, parsed)
	assert.Empty(t, noMatch)

	matchedZero, parsed := expvarSeriesFromPromText(`# TYPE dogstatsd__processed counter
dogstatsd__processed{message_type="metrics",state="ok"} 0
`)
	require.True(t, parsed)
	require.Len(t, matchedZero, 1)
	assert.Equal(t, "dogstatsd_metric_packets", matchedZero[0].mappingID)
	assert.Zero(t, matchedZero[0].value)
}

func TestExpvarMappingsDeclareLabelPolicy(t *testing.T) {
	seen := make(map[string]struct{}, len(expvarMappings))
	for _, mapping := range expvarMappings {
		_, duplicate := seen[mapping.id]
		require.Falsef(t, duplicate, "duplicate mapping id %q", mapping.id)
		seen[mapping.id] = struct{}{}

		if len(mapping.selectors) == 0 {
			require.Truef(t, mapping.matchAllLabelSets, "mapping %q must opt into matching all label sets", mapping.id)
		} else {
			require.Falsef(t, mapping.matchAllLabelSets, "mapping %q has both selectors and match-all enabled", mapping.id)
		}
	}
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

func TestExpvarMappingsResolveInAgentComponents(t *testing.T) {
	initializeAgentExpvars(t)
	for _, mapping := range expvarMappings {
		parent, leaf, err := resolveExpvarMappingTarget(mapping)
		require.NoErrorf(t, err, "mapping %q did not resolve", mapping.id)
		require.NotNil(t, parent)
		require.NotNil(t, leaf)
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
