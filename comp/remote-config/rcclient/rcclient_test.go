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
}

func (m *mockLogLevelRuntimeSettings) Get() (interface{}, error) {
	return m.logLevel, nil
}

func (m *mockLogLevelRuntimeSettings) Set(v interface{}) error {
	if m.expectedError != nil {
		return m.expectedError
	}
	m.logLevel = v.(string)
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

func TestAgentConfigCallback(t *testing.T) {
	pkglog.SetupLogger(seelog.Default, "info")
	mockSettings := &mockLogLevelRuntimeSettings{}
	err := settings.RegisterRuntimeSetting(mockSettings)
	assert.NoError(t, err)

	rc := fxutil.Test[Component](t, fx.Options(Module, log.MockModule))

	layer := state.RawConfig{Config: []byte(`{"name": "layer1", "config": {"log_level": "debug"}}`)}
	configOrder := state.RawConfig{Config: []byte(`{"internal_order": ["layer1", "layer2"]}`)}

	structRC := rc.(rcClient)

	structRC.client, _ = remote.NewUnverifiedGRPCClient(
		"test-agent",
		"9.99.9",
		[]data.Product{data.ProductAgentConfig},
		1*time.Hour,
	)

	// Set log level to debug
	structRC.agentConfigUpdateCallback(map[string]state.RawConfig{
		"datadog/2/AGENT_CONFIG/layer1/configname":              layer,
		"datadog/2/AGENT_CONFIG/configuration_order/configname": configOrder,
	})
	assert.Equal(t, "debug", mockSettings.logLevel)
}
