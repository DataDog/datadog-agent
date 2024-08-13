// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package mock offers a mock implementation for the configuration
package mock

import (
	"strings"
	"sync"
	"testing"

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
	model.Config
}

// Set is used for setting configuration in tests
func (c *mockConfig) Set(key string, value interface{}, source model.Source) {
	c.Config.Set(key, value, source)
}

// SetWithoutSource is used for setting configuration in tests
func (c *mockConfig) SetWithoutSource(key string, value interface{}) {
	c.Config.SetWithoutSource(key, value)
}

// SetKnown is used for setting configuration in tests
func (c *mockConfig) SetKnown(key string) {
	c.Config.SetKnown(key)
}

// New creates a mock for the config
func New(t testing.TB) model.Config {
	// We only check isConfigMocked when registering a cleanup function. 'isConfigMocked' avoids nested calls to
	// Mock to reset the config to a blank state. This way we have only one mock per test and test helpers can call
	// Mock.
	if t != nil {
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
			setup.SetDatadog(originalDatadogConfig)
		})
	}

	// Configure Datadog global configuration
	newCfg := model.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	// Configuration defaults
	setup.SetDatadog(newCfg)
	setup.InitConfig(newCfg)
	return &mockConfig{newCfg}
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
			setup.SetSystemProbe(originalConfig)
		})
	}

	// Configure Datadog global configuration
	setup.SetSystemProbe(model.NewConfig("system-probe", "DD", strings.NewReplacer(".", "_")))
	// Configuration defaults
	setup.InitSystemProbeConfig(setup.SystemProbe())
	return &mockConfig{setup.SystemProbe()}
}
