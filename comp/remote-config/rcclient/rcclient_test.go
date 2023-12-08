// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package rcclient

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
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
}

func (m *mockLogLevelRuntimeSettings) Get() (interface{}, error) {
	return m.logLevel, nil
}

func (m *mockLogLevelRuntimeSettings) Set(v interface{}, source model.Source) error {
	if m.expectedError != nil {
		return m.expectedError
	}
	m.logLevel = v.(string)
	config.Datadog.Set(m.Name(), m.logLevel, source)
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

func TestAgentConfigCallback(t *testing.T) {
	pkglog.SetupLogger(seelog.Default, "info")
	_ = config.Mock(t)
	mockSettings := &mockLogLevelRuntimeSettings{logLevel: "info"}
	err := settings.RegisterRuntimeSetting(mockSettings)
	assert.NoError(t, err)

	rc := fxutil.Test[Component](t, fx.Options(Module(), logimpl.MockModule()))

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
	assert.Equal(t, model.SourceDefault, config.Datadog.GetSource("log_level"))

	// Set log level to debug
	structRC.agentConfigUpdateCallback(map[string]state.RawConfig{
		"datadog/2/AGENT_CONFIG/layer1/configname":              layerStartFlare,
		"datadog/2/AGENT_CONFIG/configuration_order/configname": configOrder,
	}, applyEmpty)
	assert.Equal(t, "debug", config.Datadog.Get("log_level"))
	assert.Equal(t, model.SourceRC, config.Datadog.GetSource("log_level"))

	// Send an empty log level request, as RC would at the end of the Agent Flare request
	// Should fallback to the default level
	structRC.agentConfigUpdateCallback(map[string]state.RawConfig{
		"datadog/2/AGENT_CONFIG/layer1/configname":              layerEndFlare,
		"datadog/2/AGENT_CONFIG/configuration_order/configname": configOrder,
	}, applyEmpty)
	assert.Equal(t, "info", config.Datadog.Get("log_level"))
	assert.Equal(t, model.SourceDefault, config.Datadog.GetSource("log_level"))

	// -----------------
	// Test scenario #2: log level was changed by the user BEFORE Agent Flare request
	config.Datadog.Set("log_level", "info", model.SourceCLI)
	structRC.agentConfigUpdateCallback(map[string]state.RawConfig{
		"datadog/2/AGENT_CONFIG/layer1/configname":              layerStartFlare,
		"datadog/2/AGENT_CONFIG/configuration_order/configname": configOrder,
	}, applyEmpty)
	// Log level should still be "info" because it was enforced by the user
	assert.Equal(t, "info", config.Datadog.Get("log_level"))
	// Source should still be CLI as it has priority over RC
	assert.Equal(t, model.SourceCLI, config.Datadog.GetSource("log_level"))

	// -----------------
	// Test scenario #3: log level is changed by the user DURING the Agent Flare request
	config.Datadog.UnsetForSource("log_level", model.SourceCLI)
	structRC.agentConfigUpdateCallback(map[string]state.RawConfig{
		"datadog/2/AGENT_CONFIG/layer1/configname":              layerStartFlare,
		"datadog/2/AGENT_CONFIG/configuration_order/configname": configOrder,
	}, applyEmpty)
	assert.Equal(t, "debug", config.Datadog.Get("log_level"))
	assert.Equal(t, model.SourceRC, config.Datadog.GetSource("log_level"))

	config.Datadog.Set("log_level", "debug", model.SourceCLI)
	structRC.agentConfigUpdateCallback(map[string]state.RawConfig{
		"datadog/2/AGENT_CONFIG/layer1/configname":              layerEndFlare,
		"datadog/2/AGENT_CONFIG/configuration_order/configname": configOrder,
	}, applyEmpty)
	assert.Equal(t, "debug", config.Datadog.Get("log_level"))
	assert.Equal(t, model.SourceCLI, config.Datadog.GetSource("log_level"))
}
