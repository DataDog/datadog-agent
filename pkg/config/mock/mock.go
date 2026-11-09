// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package mock offers a mock implementation for the configuration
package mock

import (
	"bytes"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/create"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
)

var (
	isConfigMocked            = false
	isSystemProbeConfigMocked = false
	m                         = sync.Mutex{}
)

// mockConfig should only be used in tests
type mockConfig struct {
	model.BuildableConfig
}

// New creates a mock for the config
func New(t testing.TB) model.BuildableConfig {
	m.Lock()
	defer m.Unlock()
	if isConfigMocked {
		// The configuration is already mocked.
		return &mockConfig{setup.GlobalConfigBuilder()}
	}

	isConfigMocked = true
	originalDatadogConfig := setup.GlobalConfigBuilder()
	t.Cleanup(func() {
		m.Lock()
		defer m.Unlock()
		isConfigMocked = false
		setup.SetDatadog(originalDatadogConfig) // nolint: forbidigo // legitimate use of SetDatadog
	})

	// Configure Datadog global configuration
	newCfg := create.NewConfig("datadog")
	// Configuration defaults
	setup.SetDatadog(newCfg) // nolint forbidigo legitimate use of SetDatadog
	setup.InitConfig(newCfg)
	newCfg.BuildSchema()
	newCfg.SetTestOnlyDynamicSchema(true)
	return &mockConfig{newCfg}
}

// NewFromYAML creates a mock for the config and load the give YAML
func NewFromYAML(t testing.TB, yamlData string) model.BuildableConfig {
	conf := New(t)
	conf.SetConfigType("yaml")
	err := conf.ReadConfig(bytes.NewBuffer([]byte(yamlData)))
	require.NoError(t, err)
	return conf
}

// NewFromFile creates a mock for the config and load the give YAML
func NewFromFile(t testing.TB, yamlFilePath string) model.BuildableConfig {
	conf := New(t)
	conf.SetConfigType("yaml")
	conf.SetConfigFile(yamlFilePath)
	err := conf.ReadInConfig()
	require.NoErrorf(t, err, "error loading yaml config file '%s'", yamlFilePath)
	return conf
}

// NewSystemProbe creates a mock for the system-probe config
func NewSystemProbe(t testing.TB) model.BuildableConfig {
	// We only check isSystemProbeConfigMocked when registering a cleanup function. 'isSystemProbeConfigMocked'
	// avoids nested calls to Mock to reset the config to a blank state. This way we have only one mock per test and
	// test helpers can call Mock.
	if t != nil {
		m.Lock()
		defer m.Unlock()
		if isSystemProbeConfigMocked {
			// The configuration is already mocked.
			return &mockConfig{setup.GlobalSystemProbeConfigBuilder()}
		}

		isSystemProbeConfigMocked = true
		originalConfig := setup.GlobalSystemProbeConfigBuilder()
		t.Cleanup(func() {
			m.Lock()
			defer m.Unlock()
			isSystemProbeConfigMocked = false
			setup.SetSystemProbe(originalConfig) // nolint forbidigo legitimate use of SetSystemProbe
		})
	}

	// Configure Datadog global configuration
	setup.SetSystemProbe(create.NewConfig("system-probe")) // nolint forbidigo legitimate use of SetSystemProbe
	// Configuration defaults
	setup.InitSystemProbeConfig(setup.GlobalSystemProbeConfigBuilder())
	setup.SystemProbe().SetTestOnlyDynamicSchema(true)
	return &mockConfig{setup.GlobalSystemProbeConfigBuilder()}
}

// SetDefaultConfigType sets the config type for the mock config in use
func SetDefaultConfigType(t *testing.T, configType string) {
	mockConfig := New(t)
	mockConfig.SetConfigType(configType)
}
