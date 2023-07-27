// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package rcclient

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/config/remote"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
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
	Source        settings.LogLevelSource
}

func (m *mockLogLevelRuntimeSettings) Get() (interface{}, error) {
	return m.logLevel, nil
}

func (m *mockLogLevelRuntimeSettings) Set(v interface{}, source settings.LogLevelSource) error {
	if m.expectedError != nil {
		return m.expectedError
	}
	m.logLevel = v.(string)
	m.Source = source
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

func (m *mockLogLevelRuntimeSettings) GetSource() settings.LogLevelSource {
	return m.Source
}

func TestAgentConfigCallback(t *testing.T) {
	pkglog.SetupLogger(seelog.Default, "info")
	mockSettings := &mockLogLevelRuntimeSettings{logLevel: "info", Source: settings.LogLevelSourceDefault}
	err := settings.RegisterRuntimeSetting(mockSettings)
	assert.NoError(t, err)

	rc := fxutil.Test[Component](t, fx.Options(Module, log.MockModule))

	layerStartFlare := state.RawConfig{Config: []byte(`{"name": "layer1", "config": {"log_level": "debug"}}`)}
	layerEndFlare := state.RawConfig{Config: []byte(`{"name": "layer1", "config": {"log_level": ""}}`)}
	configOrder := state.RawConfig{Config: []byte(`{"internal_order": ["layer1", "layer2"]}`)}

	structRC := rc.(rcClient)

	structRC.client, _ = remote.NewUnverifiedGRPCClient(
		"test-agent",
		"9.99.9",
		[]data.Product{data.ProductAgentConfig},
		1*time.Hour,
	)

	// -----------------
	// Test scenario #1: Agent Flare request by RC and the log level hadn't been changed by the user before
	assert.Equal(t, settings.LogLevelSourceDefault, mockSettings.Source)

	// Set log level to debug
	structRC.agentConfigUpdateCallback(map[string]state.RawConfig{
		"datadog/2/AGENT_CONFIG/layer1/configname":              layerStartFlare,
		"datadog/2/AGENT_CONFIG/configuration_order/configname": configOrder,
	})
	assert.Equal(t, "debug", mockSettings.logLevel)
	assert.Equal(t, settings.LogLevelSourceRC, mockSettings.Source)

	// Send an empty log level request, as RC would at the end of the Agent Flare request
	// Should fallback to the default level
	structRC.agentConfigUpdateCallback(map[string]state.RawConfig{
		"datadog/2/AGENT_CONFIG/layer1/configname":              layerEndFlare,
		"datadog/2/AGENT_CONFIG/configuration_order/configname": configOrder,
	})
	assert.Equal(t, "info", mockSettings.logLevel)
	assert.Equal(t, settings.LogLevelSourceDefault, mockSettings.Source)

	// -----------------
	// Test scenario #2: log level was changed by the user BEFORE Agent Flare request
	mockSettings.Source = settings.LogLevelSourceCLI
	structRC.agentConfigUpdateCallback(map[string]state.RawConfig{
		"datadog/2/AGENT_CONFIG/layer1/configname":              layerStartFlare,
		"datadog/2/AGENT_CONFIG/configuration_order/configname": configOrder,
	})
	// Log level should still be "info" because it was enforced by the user
	assert.Equal(t, "info", mockSettings.logLevel)
	// Source should still be CLI as it has priority over RC
	assert.Equal(t, settings.LogLevelSourceCLI, mockSettings.Source)

	// -----------------
	// Test scenario #3: log level is changed by the user DURING the Agent Flare request
	mockSettings.Source = settings.LogLevelSourceDefault
	structRC.agentConfigUpdateCallback(map[string]state.RawConfig{
		"datadog/2/AGENT_CONFIG/layer1/configname":              layerStartFlare,
		"datadog/2/AGENT_CONFIG/configuration_order/configname": configOrder,
	})
	assert.Equal(t, "debug", mockSettings.logLevel)
	assert.Equal(t, settings.LogLevelSourceRC, mockSettings.Source)

	mockSettings.Source = settings.LogLevelSourceCLI
	structRC.agentConfigUpdateCallback(map[string]state.RawConfig{
		"datadog/2/AGENT_CONFIG/layer1/configname":              layerEndFlare,
		"datadog/2/AGENT_CONFIG/configuration_order/configname": configOrder,
	})
	assert.Equal(t, "debug", mockSettings.logLevel)
	assert.Equal(t, settings.LogLevelSourceCLI, mockSettings.Source)
}
