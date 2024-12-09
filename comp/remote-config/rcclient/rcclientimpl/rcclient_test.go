// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package rcclientimpl

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/settings"
	"github.com/DataDog/datadog-agent/comp/core/settings/settingsimpl"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient"
	"github.com/DataDog/datadog-agent/pkg/api/security"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/remote/client"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
)

type mockLogLevelRuntimeSettings struct {
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
	pkgconfigsetup.Datadog().Set(m.Name(), m.logLevel, source)
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
		),
	)
	assert.NoError(t, err)
	assert.NotNil(t, client)
	assert.NotNil(t, client.(rcClient).client)
}

func TestAgentConfigCallback(t *testing.T) {
	pkglog.SetupLogger(seelog.Default, "info")
	cfg := configmock.New(t)

	rc := fxutil.Test[rcclient.Component](t,
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
						"log_level": &mockLogLevelRuntimeSettings{logLevel: "info"},
					},
					Config: cfg,
				},
			),
			settingsimpl.Module(),
		),
	)

	layerStartFlare := state.RawConfig{Config: []byte(`{"name": "layer1", "config": {"log_level": "debug"}}`)}
	layerEndFlare := state.RawConfig{Config: []byte(`{"name": "layer1", "config": {"log_level": ""}}`)}
	configOrder := state.RawConfig{Config: []byte(`{"internal_order": ["layer1", "layer2"]}`)}

	structRC := rc.(rcClient)

	ipcAddress, err := pkgconfigsetup.GetIPCAddress(cfg)
	assert.NoError(t, err)

	structRC.client, _ = client.NewUnverifiedGRPCClient(
		ipcAddress, pkgconfigsetup.GetIPCPort(), func() (string, error) { return security.FetchAuthToken(cfg) },
		client.WithAgent("test-agent", "9.99.9"),
		client.WithProducts(state.ProductAgentConfig),
		client.WithPollInterval(time.Hour),
	)

	// -----------------
	// Test scenario #1: Agent Flare request by RC and the log level hadn't been changed by the user before
	assert.Equal(t, model.SourceDefault, cfg.GetSource("log_level"))

	// Set log level to debug
	structRC.agentConfigUpdateCallback(map[string]state.RawConfig{
		"datadog/2/AGENT_CONFIG/layer1/configname":              layerStartFlare,
		"datadog/2/AGENT_CONFIG/configuration_order/configname": configOrder,
	}, applyEmpty)
	assert.Equal(t, "debug", cfg.Get("log_level"))
	assert.Equal(t, model.SourceRC, cfg.GetSource("log_level"))

	// Send an empty log level request, as RC would at the end of the Agent Flare request
	// Should fallback to the default level
	structRC.agentConfigUpdateCallback(map[string]state.RawConfig{
		"datadog/2/AGENT_CONFIG/layer1/configname":              layerEndFlare,
		"datadog/2/AGENT_CONFIG/configuration_order/configname": configOrder,
	}, applyEmpty)
	assert.Equal(t, "info", cfg.Get("log_level"))
	assert.Equal(t, model.SourceDefault, cfg.GetSource("log_level"))

	// -----------------
	// Test scenario #2: log level was changed by the user BEFORE Agent Flare request
	cfg.Set("log_level", "info", model.SourceCLI)
	structRC.agentConfigUpdateCallback(map[string]state.RawConfig{
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
	structRC.agentConfigUpdateCallback(map[string]state.RawConfig{
		"datadog/2/AGENT_CONFIG/layer1/configname":              layerStartFlare,
		"datadog/2/AGENT_CONFIG/configuration_order/configname": configOrder,
	}, applyEmpty)
	assert.Equal(t, "debug", cfg.Get("log_level"))
	assert.Equal(t, model.SourceRC, cfg.GetSource("log_level"))

	cfg.Set("log_level", "debug", model.SourceCLI)
	structRC.agentConfigUpdateCallback(map[string]state.RawConfig{
		"datadog/2/AGENT_CONFIG/layer1/configname":              layerEndFlare,
		"datadog/2/AGENT_CONFIG/configuration_order/configname": configOrder,
	}, applyEmpty)
	assert.Equal(t, "debug", cfg.Get("log_level"))
	assert.Equal(t, model.SourceCLI, cfg.GetSource("log_level"))
}
