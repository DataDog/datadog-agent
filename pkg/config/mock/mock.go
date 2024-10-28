// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package mock offers a mock implementation for the configuration
package mock

import (
	"bytes"
	"strings"
	"sync"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/stretchr/testify/require"
)

var (
	isConfigMocked            = false
	isSystemProbeConfigMocked = false
	m                         = sync.Mutex{}
)

// mockConfig should only be used in tests
type mockConfig struct {
	model.Config
}

// New creates a mock for the config
func New(t testing.TB) model.Config {
	m.Lock()
	defer m.Unlock()
	if isConfigMocked {
		// The configuration is already mocked.
		return &mockConfig{setup.Datadog()}
	}

	isConfigMocked = true
	originalDatadogConfig := setup.Datadog()
	t.Cleanup(func() {
		m.Lock()
		defer m.Unlock()
		isConfigMocked = false
		setup.SetDatadog(originalDatadogConfig) // nolint: forbidigo // legitimate use of SetDatadog
	})

	// Configure Datadog global configuration
	newCfg := model.NewConfig("datadog", "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo // legitimate use of NewConfig
	// Configuration defaults
	setup.SetDatadog(newCfg) // nolint forbidigo legitimate use of SetDatadog
	setup.InitConfig(newCfg)
	return &mockConfig{newCfg}
}

// NewFromYAML creates a mock for the config and load the give YAML
func NewFromYAML(t testing.TB, yamlData string) model.Config {
	conf := New(t)
	conf.SetConfigType("yaml")
	err := conf.ReadConfig(bytes.NewBuffer([]byte(yamlData)))
	require.NoError(t, err)
	return conf
}

// NewFromFile creates a mock for the config and load the give YAML
func NewFromFile(t testing.TB, yamlFilePath string) model.Config {
	conf := New(t)
	conf.SetConfigType("yaml")
	conf.SetConfigFile(yamlFilePath)
	err := conf.ReadInConfig()
	require.NoErrorf(t, err, "error loading yaml config file '%s'", yamlFilePath)
	return conf
}

// NewSystemProbe creates a mock for the system-probe config
func NewSystemProbe(t testing.TB) model.Config {
	// We only check isSystemProbeConfigMocked when registering a cleanup function. 'isSystemProbeConfigMocked'
	// avoids nested calls to Mock to reset the config to a blank state. This way we have only one mock per test and
	// test helpers can call Mock.
	if t != nil {
		m.Lock()
		defer m.Unlock()
		if isSystemProbeConfigMocked {
			// The configuration is already mocked.
			return &mockConfig{setup.SystemProbe()}
		}

		isSystemProbeConfigMocked = true
		originalConfig := setup.SystemProbe()
		t.Cleanup(func() {
			m.Lock()
			defer m.Unlock()
			isSystemProbeConfigMocked = false
			setup.SetSystemProbe(originalConfig) // nolint forbidigo legitimate use of SetSystemProbe
		})
	}

	// Configure Datadog global configuration
	setup.SetSystemProbe(model.NewConfig("system-probe", "DD", strings.NewReplacer(".", "_"))) // nolint forbidigo legitimate use of NewConfig and SetSystemProbe
	// Configuration defaults
	setup.InitSystemProbeConfig(setup.SystemProbe())
	return &mockConfig{setup.SystemProbe()}
}
