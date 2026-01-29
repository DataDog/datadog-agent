// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package rcclientimpl

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/settings"
	"github.com/DataDog/datadog-agent/comp/core/settings/settingsimpl"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/remote/client"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
)

type mockLogLevelRuntimeSettings struct {
	cfg           config.Component
	expectedError error
	logLevel      string
}

func (m *mockLogLevelRuntimeSettings) Get(_ config.Component) (interface{}, error) {
	return m.logLevel, nil
}

func (m *mockLogLevelRuntimeSettings) Set(_ config.Component, v interface{}, source model.Source) error {
	if m.expectedError != nil {
		return m.expectedError
	}
	m.logLevel = v.(string)
	m.cfg.Set(m.Name(), m.logLevel, source)
	return nil
}

func (m *mockLogLevelRuntimeSettings) Name() string {
	return "log_level"
}

func (m *mockLogLevelRuntimeSettings) Description() string {
	return ""
}

func (m *mockLogLevelRuntimeSettings) Hidden() bool {
	return true
}

func applyEmpty(_ string, _ state.ApplyStatus) {}

func TestRCClientCreate(t *testing.T) {
	_, err := newRemoteConfigClient(
		fxutil.Test[dependencies](
			t,
			fx.Provide(func() log.Component { return logmock.New(t) }),
			fx.Provide(func() config.Component { return configmock.New(t) }),
			settingsimpl.MockModule(),
			sysprobeconfig.NoneModule(),
			fx.Provide(func() ipc.Component { return ipcmock.New(t) }),
		),
	)
	// Missing params
	assert.Error(t, err)

	client, err := newRemoteConfigClient(
		fxutil.Test[dependencies](
			t,
			fx.Provide(func() log.Component { return logmock.New(t) }),
			fx.Provide(func() config.Component { return configmock.New(t) }),
			sysprobeconfig.NoneModule(),
			fx.Supply(
				rcclient.Params{
					AgentName:    "test-agent",
					AgentVersion: "7.0.0",
				},
			),
			settingsimpl.MockModule(),
			fx.Provide(func() ipc.Component { return ipcmock.New(t) }),
		),
	)
	assert.NoError(t, err)
	assert.NotNil(t, client)
	assert.NotNil(t, client.(*rcClient).client)
}

func TestAgentConfigCallback(t *testing.T) {
	pkglog.SetupLogger(pkglog.Default(), "info")
	cfg := configmock.New(t)

	var ipcComp ipc.Component

	rcComponent := fxutil.Test[rcclient.Component](t,
		fx.Options(
			Module(),
			fx.Provide(func() log.Component { return logmock.New(t) }),
			fx.Provide(func() config.Component { return cfg }),
			sysprobeconfig.NoneModule(),
			fx.Supply(
				rcclient.Params{
					AgentName:    "test-agent",
					AgentVersion: "7.0.0",
				},
			),
			fx.Supply(
				settings.Params{
					Settings: map[string]settings.RuntimeSetting{
						"log_level": &mockLogLevelRuntimeSettings{cfg: cfg, logLevel: "info"},
					},
					Config: cfg,
				},
			),
			settingsimpl.Module(),
			fx.Provide(func() ipc.Component { return ipcmock.New(t) }),
			fx.Populate(&ipcComp),
		),
	)

	layerStartFlare := state.RawConfig{Config: []byte(`{"name": "layer1", "config": {"log_level": "debug"}}`)}
	layerEndFlare := state.RawConfig{Config: []byte(`{"name": "layer1", "config": {"log_level": ""}}`)}
	configOrder := state.RawConfig{Config: []byte(`{"internal_order": ["layer1", "layer2"]}`)}

	rc := rcComponent.(*rcClient)

	ipcAddress, err := pkgconfigsetup.GetIPCAddress(cfg)
	assert.NoError(t, err)

	rc.client, _ = client.NewUnverifiedGRPCClient(
		ipcAddress,
		pkgconfigsetup.GetIPCPort(),
		ipcComp.GetAuthToken(),
		ipcComp.GetTLSClientConfig(),
		client.WithAgent("test-agent", "9.99.9"),
		client.WithProducts(state.ProductAgentConfig),
		client.WithPollInterval(time.Hour),
	)

	// -----------------
	// Test scenario #1: Agent Flare request by RC and the log level hadn't been changed by the user before
	assert.Equal(t, model.SourceDefault, cfg.GetSource("log_level"))

	// Set log level to debug
	rc.agentConfigUpdateCallback(map[string]state.RawConfig{
		"datadog/2/AGENT_CONFIG/layer1/configname":              layerStartFlare,
		"datadog/2/AGENT_CONFIG/configuration_order/configname": configOrder,
	}, applyEmpty)
	assert.Equal(t, "debug", cfg.Get("log_level"))
	assert.Equal(t, model.SourceRC, cfg.GetSource("log_level"))

	// Send an empty log level request, as RC would at the end of the Agent Flare request
	// Should fallback to the default level
	rc.agentConfigUpdateCallback(map[string]state.RawConfig{
		"datadog/2/AGENT_CONFIG/layer1/configname":              layerEndFlare,
		"datadog/2/AGENT_CONFIG/configuration_order/configname": configOrder,
	}, applyEmpty)
	assert.Equal(t, "info", cfg.Get("log_level"))
	assert.Equal(t, model.SourceDefault, cfg.GetSource("log_level"))

	// -----------------
	// Test scenario #2: log level was changed by the user BEFORE Agent Flare request
	cfg.Set("log_level", "info", model.SourceCLI)
	rc.agentConfigUpdateCallback(map[string]state.RawConfig{
		"datadog/2/AGENT_CONFIG/layer1/configname":              layerStartFlare,
		"datadog/2/AGENT_CONFIG/configuration_order/configname": configOrder,
	}, applyEmpty)
	// Log level should still be "info" because it was enforced by the user
	assert.Equal(t, "info", cfg.Get("log_level"))
	// Source should still be CLI as it has priority over RC
	assert.Equal(t, model.SourceCLI, cfg.GetSource("log_level"))

	// -----------------
	// Test scenario #3: log level is changed by the user DURING the Agent Flare request
	cfg.UnsetForSource("log_level", model.SourceCLI)
	rc.agentConfigUpdateCallback(map[string]state.RawConfig{
		"datadog/2/AGENT_CONFIG/layer1/configname":              layerStartFlare,
		"datadog/2/AGENT_CONFIG/configuration_order/configname": configOrder,
	}, applyEmpty)
	assert.Equal(t, "debug", cfg.Get("log_level"))
	assert.Equal(t, model.SourceRC, cfg.GetSource("log_level"))

	cfg.Set("log_level", "debug", model.SourceCLI)
	rc.agentConfigUpdateCallback(map[string]state.RawConfig{
		"datadog/2/AGENT_CONFIG/layer1/configname":              layerEndFlare,
		"datadog/2/AGENT_CONFIG/configuration_order/configname": configOrder,
	}, applyEmpty)
	assert.Equal(t, "debug", cfg.Get("log_level"))
	assert.Equal(t, model.SourceCLI, cfg.GetSource("log_level"))
}

func TestAgentMRFConfigCallback(t *testing.T) {
	pkglog.SetupLogger(pkglog.Default(), "info")
	cfg := configmock.New(t)

	var ipcComp ipc.Component
	var settingsComp settings.Component

	rcComponent := fxutil.Test[rcclient.Component](t,
		fx.Options(
			Module(),
			fx.Provide(func() log.Component { return logmock.New(t) }),
			fx.Provide(func() config.Component { return cfg }),
			sysprobeconfig.NoneModule(),
			fx.Supply(
				rcclient.Params{
					AgentName:    "test-agent",
					AgentVersion: "7.0.0",
				},
			),
			settingsimpl.MockModule(),
			fx.Provide(func() ipc.Component { return ipcmock.New(t) }),
			fx.Populate(&ipcComp),
			fx.Populate(&settingsComp),
		),
	)

	allInactive := state.RawConfig{Config: []byte(`{"name": "none"}`)}
	noLogs := state.RawConfig{Config: []byte(`{"name": "nologs", "failover_logs": false}`)}
	activeMetrics := state.RawConfig{Config: []byte(`{"name": "yesmetrics", "failover_metrics": true}`)}
	activeAPM := state.RawConfig{Config: []byte(`{"name": "yesapm", "failover_apm": true}`)}
	activeAllowlist := state.RawConfig{Config: []byte(`{"name": "yesallowlist", "metrics_allowlist": ["system.cpu.usage"]}`)}
	emptyAllowlist := state.RawConfig{Config: []byte(`{"name": "emptyallowlist", "metrics_allowlist": []}`)}
	nilAllowlist := state.RawConfig{Config: []byte(`{"name": "nilallowlist"}`)}

	rc := rcComponent.(*rcClient)

	ipcAddress, err := pkgconfigsetup.GetIPCAddress(cfg)
	assert.NoError(t, err)

	rc.client, _ = client.NewUnverifiedGRPCClient(
		ipcAddress,
		pkgconfigsetup.GetIPCPort(),
		ipcComp.GetAuthToken(),
		ipcComp.GetTLSClientConfig(),
		client.WithAgent("test-agent", "9.99.9"),
		client.WithProducts(state.ProductAgentConfig),
		client.WithPollInterval(time.Hour),
	)

	// Should enable metrics failover and disable logs failover
	// and set the metrics allowlist
	rc.mrfUpdateCallback(map[string]state.RawConfig{
		"datadog/2/AGENT_FAILOVER/none/configname":         allInactive,
		"datadog/2/AGENT_FAILOVER/nologs/configname":       noLogs,
		"datadog/2/AGENT_FAILOVER/yesmetrics/configname":   activeMetrics,
		"datadog/2/AGENT_FAILOVER/yesapm/configname":       activeAPM,
		"datadog/2/AGENT_FAILOVER/yesallowlist/configname": activeAllowlist,
	}, applyEmpty)

	metricsVal, _ := settingsComp.GetRuntimeSetting("multi_region_failover.failover_metrics")
	logsVal, _ := settingsComp.GetRuntimeSetting("multi_region_failover.failover_logs")
	apmVal, _ := settingsComp.GetRuntimeSetting("multi_region_failover.failover_apm")
	allowlistVal, _ := settingsComp.GetRuntimeSetting("multi_region_failover.metric_allowlist")
	assert.True(t, metricsVal.(bool))
	assert.False(t, logsVal.(bool))
	assert.True(t, apmVal.(bool))
	assert.ElementsMatch(t, []string{"system.cpu.usage"}, allowlistVal.([]string))

	// Should set an empty allowlist
	rc.mrfUpdateCallback(map[string]state.RawConfig{
		"datadog/2/AGENT_FAILOVER/yesallowlist/configname": emptyAllowlist,
		"datadog/2/AGENT_FAILOVER/yesmetrics/configname":   activeMetrics,
	}, applyEmpty)

	metricsVal, _ = settingsComp.GetRuntimeSetting("multi_region_failover.failover_metrics")
	allowlistVal, _ = settingsComp.GetRuntimeSetting("multi_region_failover.metric_allowlist")
	assert.True(t, metricsVal.(bool))
	assert.ElementsMatch(t, []string{}, allowlistVal.([]string))

	// Should not set an allowlist (nil means not configured, so we fallback)
	// First, let's set a new mock to verify allowlist is not set
	settingsComp2 := fxutil.Test[settings.Component](t, settingsimpl.MockModule())
	rc.settingsComponent = settingsComp2
	rc.mrfUpdateCallback(map[string]state.RawConfig{
		"datadog/2/AGENT_FAILOVER/emptyallowlist/configname": nilAllowlist,
		"datadog/2/AGENT_FAILOVER/yesmetrics/configname":     activeMetrics,
	}, applyEmpty)

	metricsVal, _ = settingsComp2.GetRuntimeSetting("multi_region_failover.failover_metrics")
	allowlistVal, _ = settingsComp2.GetRuntimeSetting("multi_region_failover.metric_allowlist")
	assert.True(t, metricsVal.(bool))
	assert.Nil(t, allowlistVal)
}
