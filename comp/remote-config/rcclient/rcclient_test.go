// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package rcclient

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/remote/client"
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

	ipcAddress, err := config.GetIPCAddress()
	assert.NoError(t, err)

	structRC.client, _ = client.NewUnverifiedGRPCClient(
		ipcAddress, config.GetIPCPort(), security.FetchAuthToken,
		client.WithAgent("test-agent", "9.99.9"),
		client.WithProducts([]data.Product{data.ProductAgentConfig}),
		client.WithPollInterval(time.Hour),
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

func TestOnAPMTracingUpdate(t *testing.T) {
	mkTemp := func(t *testing.T) func() {
		oldPath := apmTracingFilePath
		f, err := os.CreateTemp("", "test")
		require.NoError(t, err)
		apmTracingFilePath = f.Name()
		return func() {
			apmTracingFilePath = oldPath
		}
	}

	t.Run("Empty update deletes file", func(t *testing.T) {
		defer mkTemp(t)()
		rc := rcClient{}

		rc.onAPMTracingUpdate(map[string]state.RawConfig{}, nil)

		_, err := os.Open(apmTracingFilePath)
		if !os.IsNotExist(err) {
			// file still exists when it shouldn't
			assert.Fail(t, "Empty update did not delete existing config file")
		}
	})

	t.Run("Valid update writes file", func(t *testing.T) {
		defer mkTemp(t)()
		rc := rcClient{}
		callbackCalls := map[string]string{}
		callback := func(id string, status state.ApplyStatus) {
			callbackCalls[id] = status.Error
		}

		hostConfig := state.RawConfig{Config: []byte(`{"infra_target": {"tags":["k:v"]},"lib_config":{"env":"someEnv","tracing_enabled":true}}`)}
		senvConfig := state.RawConfig{Config: []byte(`{"service_target": {"service":"s1", "env":"e1"}}`)}

		updates := map[string]state.RawConfig{
			"host1": hostConfig,
			"srv1":  senvConfig,
		}
		rc.onAPMTracingUpdate(updates, callback)

		assert.Len(t, callbackCalls, 2)
		assert.Empty(t, callbackCalls["host1"])
		assert.Empty(t, callbackCalls["srv1"])
		actualBytes, err := os.ReadFile(apmTracingFilePath)
		assert.NoError(t, err)
		assert.Equal(t, "tracing_enabled: true\nenv: someEnv\nservice_env_configs:\n- service: s1\n  env: e1\n  tracing_enabled: false\n", string(actualBytes))
	})

	t.Run("bad updates report failure", func(t *testing.T) {
		defer mkTemp(t)()
		rc := rcClient{}
		calls := map[string]string{}
		callback := func(id string, status state.ApplyStatus) {
			calls[id] = status.Error
		}

		missingTarget := state.RawConfig{Config: []byte(`{}`)}
		badPayload := state.RawConfig{Config: []byte(`{`)}

		updates := map[string]state.RawConfig{
			"missingTarget": missingTarget,
			"badPayload":    badPayload,
		}
		rc.onAPMTracingUpdate(updates, callback)

		assert.Len(t, calls, 2)
		assert.Equal(t, calls["missingTarget"], MissingServiceTarget)
		assert.Equal(t, calls["badPayload"], InvalidAPMTracingPayload)
	})
}
