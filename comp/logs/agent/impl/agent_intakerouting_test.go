// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test && !serverless

package agentimpl

import (
	"context"
	"maps"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
	"go.uber.org/fx"

	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	secretsnoopimpl "github.com/DataDog/datadog-agent/comp/core/secrets/noop-impl"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	intakerouting "github.com/DataDog/datadog-agent/comp/intakerouting/def"
	intakeroutingmock "github.com/DataDog/datadog-agent/comp/intakerouting/mock"
	kubehealthmock "github.com/DataDog/datadog-agent/comp/logs-library/kubehealth/mock"
	logsmetrics "github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	flareController "github.com/DataDog/datadog-agent/comp/logs/agent/flare"
	auditorfx "github.com/DataDog/datadog-agent/comp/logs/auditor/fx"
	integrationsimpl "github.com/DataDog/datadog-agent/comp/logs/integrations/impl"
	inventoryagentmock "github.com/DataDog/datadog-agent/comp/metadata/inventoryagent/mock"
	compressionfx "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	logsStatus "github.com/DataDog/datadog-agent/pkg/logs/status"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	routingTestAPIKey = "0123456789abcdef0123456789abcdef"
	routedHost        = "acme.agent-http-intake.logs.datadoghq.com"
	otherRoutedHost   = "shard2.agent-http-intake.logs.datadoghq.com"
)

// newIntakeRoutingTestAgent returns a logs agent that is not started yet. Its endpoints point at
// the official intake, but no log is sent: no source is added.
func newIntakeRoutingTestAgent(t *testing.T, routing intakerouting.Component, overrides map[string]interface{}) *logAgent {
	cfgOverrides := map[string]interface{}{
		"logs_enabled":                  true,
		"api_key":                       routingTestAPIKey,
		"logs_config.run_path":          t.TempDir(),
		"logs_config.stop_grace_period": 1,
		// Never start the TCP to HTTP upgrade loop, which would reach the network.
		"logs_config.http_connectivity_retry_interval_max": 0,
	}
	maps.Copy(cfgOverrides, overrides)

	deps := fxutil.Test[testDeps](t,
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() configComponent.Component {
			return configComponent.NewMockWithOverrides(t, cfgOverrides)
		}),
		hostnameimpl.MockModule(),
		inventoryagentmock.MockModule(),
		auditorfx.Module(),
		fxutil.ProvideComponentConstructor(kubehealthmock.NewProvides),
	)

	agent := &logAgent{
		log:              deps.Log,
		config:           deps.Config,
		inventoryAgent:   deps.InventoryAgent,
		started:          atomic.NewUint32(0),
		integrationsLogs: integrationsimpl.NewLogsIntegration(),
		auditor:          deps.Auditor,
		sources:          sources.NewLogSources(),
		services:         service.NewServices(),
		tracker:          tailers.NewTailerTracker(),
		tagger:           taggerfxmock.SetupFakeTagger(t),
		flarecontroller:  flareController.NewFlareController(),
		compression:      compressionfx.NewMockCompressor(),
		secrets:          secretsnoopimpl.NewComponent().Comp,
		wmeta:            option.None[workloadmeta.Component](),
		intakeRouting:    routing,
	}
	t.Cleanup(func() {
		// A second stop would stop components that are not idempotent.
		agent.restartMutex.Lock()
		stopped := agent.stopped
		agent.restartMutex.Unlock()
		if !stopped {
			_ = agent.stop(context.Background())
		}
		agent.auditor.Stop()
		logsStatus.Clear()
	})
	return agent
}

func buildHTTPTestEndpoints(t *testing.T, agent *logAgent) *config.Endpoints {
	t.Helper()
	endpoints, err := buildHTTPEndpointsForRestart(agent.config)
	require.NoError(t, err)
	require.True(t, endpoints.UseHTTP)
	return endpoints
}

func startOn(t *testing.T, agent *logAgent, endpoints *config.Endpoints) {
	t.Helper()
	agent.restartMutex.Lock()
	defer agent.restartMutex.Unlock()
	require.NoError(t, agent.startLocked(context.Background(), endpoints))
}

// mainHost returns the host the pipeline currently sends to.
func mainHost(agent *logAgent) string {
	agent.restartMutex.Lock()
	defer agent.restartMutex.Unlock()
	return agent.endpoints.Main.Host
}

func successfulHTTPRestarts() float64 {
	return logsmetrics.TlmRestartAttempt.WithValues(restartStatusSuccess, transportHTTP).Get()
}

func TestIntakeRoutingAtStart(t *testing.T) {
	routing := intakeroutingmock.New()
	agent := newIntakeRoutingTestAgent(t, routing, nil)
	base := buildHTTPTestEndpoints(t, agent)
	configuredHost := base.Main.Host
	routing.SetRoute(intakerouting.Logs, configuredHost, &intakerouting.Endpoint{Host: routedHost})

	startOn(t, agent, base)

	agent.restartMutex.Lock()
	endpoints := agent.endpoints
	agent.restartMutex.Unlock()
	assert.Equal(t, routedHost, endpoints.Main.Host)
	assert.Equal(t, routedHost, endpoints.Endpoints[0].Host)
	assert.Equal(t, configuredHost, base.Main.Host, "the configured endpoints are not modified")
	assert.Contains(t, endpoints.GetStatus()[0], "(intake routing; default host: "+configuredHost+")")

	requests := routing.Requests()
	require.Len(t, requests, 1)
	assert.Equal(t, intakerouting.Logs, requests[0].Type)
	assert.Equal(t, intakerouting.Endpoint{Host: configuredHost}, requests[0].Default)
	key, ready := requests[0].APIKey()
	assert.Equal(t, routingTestAPIKey, key)
	assert.True(t, ready)
}

func TestIntakeRoutingChangesRestartThePipeline(t *testing.T) {
	routing := intakeroutingmock.New()
	agent := newIntakeRoutingTestAgent(t, routing, nil)
	base := buildHTTPTestEndpoints(t, agent)
	configuredHost := base.Main.Host
	startOn(t, agent, base)
	require.Same(t, base, agent.endpoints)

	agent.restartMutex.Lock()
	originalSources := agent.sources
	originalAuditor := agent.auditor
	originalSchedulers := agent.schedulers
	originalReceiver := agent.diagnosticMessageReceiver
	originalPipeline := agent.pipelineProvider
	agent.restartMutex.Unlock()
	restarts := successfulHTTPRestarts()

	routing.SetRoute(intakerouting.Logs, configuredHost, &intakerouting.Endpoint{Host: routedHost})
	require.Eventually(t, func() bool { return mainHost(agent) == routedHost }, 10*time.Second, 10*time.Millisecond)
	assert.Equal(t, restarts+1, successfulHTTPRestarts())

	agent.restartMutex.Lock()
	assert.Same(t, originalSources, agent.sources)
	assert.Same(t, originalAuditor, agent.auditor)
	assert.Same(t, originalSchedulers, agent.schedulers)
	assert.Same(t, originalReceiver, agent.diagnosticMessageReceiver)
	assert.NotSame(t, originalPipeline, agent.pipelineProvider)
	agent.restartMutex.Unlock()

	// Clearing the route reverts to the configured host.
	routing.SetRoute(intakerouting.Logs, configuredHost, nil)
	require.Eventually(t, func() bool { return mainHost(agent) == configuredHost }, 10*time.Second, 10*time.Millisecond)
	assert.Equal(t, restarts+2, successfulHTTPRestarts())
}

func TestIntakeRoutingBurstRestartsOnce(t *testing.T) {
	routing := intakeroutingmock.New()
	agent := newIntakeRoutingTestAgent(t, routing, nil)
	base := buildHTTPTestEndpoints(t, agent)
	configuredHost := base.Main.Host
	idle := make(chan struct{}, 8)
	agent.routingHandled = func(reg intakerouting.Registration) {
		// The watcher is idle once it has handled a notification and none is pending.
		if len(reg.Changes()) == 0 {
			idle <- struct{}{}
		}
	}
	startOn(t, agent, base)
	restarts := successfulHTTPRestarts()

	// Notifications that arrive while the pipeline is busy are coalesced.
	agent.restartMutex.Lock()
	for _, host := range []string{routedHost, otherRoutedHost, routedHost, otherRoutedHost} {
		routing.SetRoute(intakerouting.Logs, configuredHost, &intakerouting.Endpoint{Host: host})
	}
	agent.restartMutex.Unlock()

	// Every notification, including a coalesced one handled after the restart, has been handled.
	select {
	case <-idle:
	case <-time.After(10 * time.Second):
		t.Fatal("the intake routing watcher did not go idle")
	}
	assert.Equal(t, otherRoutedHost, mainHost(agent))
	assert.Equal(t, restarts+1, successfulHTTPRestarts())
}

func TestIntakeRoutingIgnoredAfterStop(t *testing.T) {
	routing := intakeroutingmock.New()
	agent := newIntakeRoutingTestAgent(t, routing, nil)
	base := buildHTTPTestEndpoints(t, agent)
	configuredHost := base.Main.Host
	startOn(t, agent, base)

	agent.restartMutex.Lock()
	reg := agent.routing.reg
	agent.restartMutex.Unlock()
	require.NotNil(t, reg)

	require.NoError(t, agent.stop(context.Background()))
	agent.restartMutex.Lock()
	assert.Nil(t, agent.routing.reg)
	agent.restartMutex.Unlock()
	restarts := successfulHTTPRestarts()

	routing.SetRoute(intakerouting.Logs, configuredHost, &intakerouting.Endpoint{Host: routedHost})
	agent.onIntakeRoutingChange(reg)
	assert.Equal(t, configuredHost, mainHost(agent))
	assert.Equal(t, restarts, successfulHTTPRestarts())

	agent.restartMutex.Lock()
	defer agent.restartMutex.Unlock()
	assert.Error(t, agent.restartLocked(base), "a stopped agent cannot be restarted")
}

func TestIntakeRoutingAdditionalEndpoints(t *testing.T) {
	const (
		otherOrgKey = "fedcba9876543210fedcba9876543210"
		euKey       = "00112233445566778899aabbccddeeff"
		euHost      = "agent-http-intake.logs.datadoghq.eu"
		euRouted    = "globex.agent-http-intake.logs.datadoghq.eu"
	)
	routing := intakeroutingmock.New()
	agent := newIntakeRoutingTestAgent(t, routing, map[string]interface{}{
		// Additional endpoints are sent over TCP unless HTTP is forced.
		"logs_config.force_use_http": true,
		"logs_config.additional_endpoints": `[
			{"api_key": "` + otherOrgKey + `", "host": "agent-http-intake.logs.datadoghq.com"},
			{"api_key": "` + euKey + `", "host": "` + euHost + `"},
			{"api_key": "` + euKey + `", "host": "logs.example.com"}
		]`,
	})
	base := buildHTTPTestEndpoints(t, agent)
	require.Len(t, base.Endpoints, 4)
	configuredHost := base.Main.Host
	routing.SetRoute(intakerouting.Logs, euHost, &intakerouting.Endpoint{Host: euRouted})

	startOn(t, agent, base)

	// Every official intake endpoint is registered with its own API key, so that each organization
	// is routed on its own. The custom host is not registered.
	requests := routing.Requests()
	require.Len(t, requests, 3)
	for i, expected := range []struct{ host, key string }{
		{configuredHost, routingTestAPIKey},
		{"agent-http-intake.logs.datadoghq.com", otherOrgKey},
		{euHost, euKey},
	} {
		assert.Equal(t, intakerouting.Endpoint{Host: expected.host}, requests[i].Default)
		key, ready := requests[i].APIKey()
		assert.Equal(t, expected.key, key)
		assert.True(t, ready)
	}

	// Only the routed endpoint changes host.
	agent.restartMutex.Lock()
	endpoints := agent.endpoints
	agent.restartMutex.Unlock()
	hosts := make([]string, len(endpoints.Endpoints))
	for i, e := range endpoints.Endpoints {
		hosts[i] = e.Host
	}
	assert.Equal(t, []string{configuredHost, "agent-http-intake.logs.datadoghq.com", euRouted, "logs.example.com"}, hosts)

	// A change for one endpoint restarts the pipeline on the new hosts.
	routing.SetRoute(intakerouting.Logs, euHost, nil)
	require.Eventually(t, func() bool {
		agent.restartMutex.Lock()
		defer agent.restartMutex.Unlock()
		return agent.endpoints.Endpoints[2].Host == euHost
	}, 10*time.Second, 10*time.Millisecond)
}

func TestIntakeRoutingSkipsTCP(t *testing.T) {
	tests := []struct {
		name      string
		overrides map[string]interface{}
	}{
		{
			name:      "forced",
			overrides: map[string]interface{}{"logs_config.force_use_tcp": true},
		},
		{
			// Additional endpoints make the logs agent use TCP unless HTTP is forced, even when
			// they are HTTP intake hosts. Then no endpoint is routed, not even the main one.
			name: "additional endpoints",
			overrides: map[string]interface{}{
				"logs_config.additional_endpoints": `[{"api_key": "fedcba9876543210fedcba9876543210", "host": "agent-http-intake.logs.datadoghq.eu"}]`,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			routing := intakeroutingmock.New()
			agent := newIntakeRoutingTestAgent(t, routing, tc.overrides)
			endpoints, err := config.BuildEndpointsWithVectorOverride(agent.config, config.HTTPConnectivitySuccess, intakeTrackType, config.AgentJSONIntakeProtocol, config.DefaultIntakeOrigin)
			require.NoError(t, err)
			require.False(t, endpoints.UseHTTP)

			startOn(t, agent, endpoints)
			agent.restartMutex.Lock()
			defer agent.restartMutex.Unlock()
			assert.Same(t, endpoints, agent.endpoints)
			assert.Empty(t, routing.Requests())
			assert.Nil(t, agent.routing.reg)
		})
	}
}

func TestIntakeRoutingAfterHTTPUpgrade(t *testing.T) {
	routing := intakeroutingmock.New()
	agent := newIntakeRoutingTestAgent(t, routing, nil)
	tcpEndpoints, err := config.BuildEndpointsWithVectorOverride(agent.config, config.HTTPConnectivityFailure, intakeTrackType, config.AgentJSONIntakeProtocol, config.DefaultIntakeOrigin)
	require.NoError(t, err)
	require.False(t, tcpEndpoints.UseHTTP)
	startOn(t, agent, tcpEndpoints)
	require.Empty(t, routing.Requests())

	configuredHost := buildHTTPTestEndpoints(t, agent).Main.Host
	routing.SetRoute(intakerouting.Logs, configuredHost, &intakerouting.Endpoint{Host: routedHost})
	require.NoError(t, agent.restartWithHTTPUpgrade(context.Background()))
	assert.Equal(t, routedHost, mainHost(agent))
	assert.Len(t, routing.Requests(), 1)

	// The new registration is watched.
	routing.SetRoute(intakerouting.Logs, configuredHost, nil)
	require.Eventually(t, func() bool { return mainHost(agent) == configuredHost }, 10*time.Second, 10*time.Millisecond)
}
