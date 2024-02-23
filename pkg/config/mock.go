// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"strings"
	"sync"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

var (
	isConfigMocked            = false
	isSystemProbeConfigMocked = false
	m                         = sync.Mutex{}
)

// MockConfig should only be used in tests
type MockConfig struct {
	Config
}

// Set is used for setting configuration in tests
func (c *MockConfig) Set(key string, value interface{}, source model.Source) {
	c.Config.Set(key, value, source)
}

// SetWithoutSource is used for setting configuration in tests
func (c *MockConfig) SetWithoutSource(key string, value interface{}) {
	c.Config.SetWithoutSource(key, value)
}

// SetKnown is used for setting configuration in tests
func (c *MockConfig) SetKnown(key string) {
	c.Config.SetKnown(key)
}

// Mock is creating and returning a mock config
func Mock(t testing.TB) *MockConfig {
	// We only check isConfigMocked when registering a cleanup function. 'isConfigMocked' avoids nested calls to
	// Mock to reset the config to a blank state. This way we have only one mock per test and test helpers can call
	// Mock.
	if t != nil {
		m.Lock()
		defer m.Unlock()
		if isConfigMocked {
			// The configuration is already mocked.
			return &MockConfig{Datadog}
		}

		isConfigMocked = true
		originalDatadogConfig := Datadog
		t.Cleanup(func() {
			m.Lock()
			defer m.Unlock()
			isConfigMocked = false
			Datadog = originalDatadogConfig
		})
	}

	// Configure Datadog global configuration
	Datadog = NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	// Configuration defaults
	pkgconfigsetup.InitConfig(Datadog)
	return &MockConfig{Datadog}
}

// MockSystemProbe is creating and returning a mock system-probe config
func MockSystemProbe(t testing.TB) *MockConfig {
	// We only check isConfigMocked when registering a cleanup function. 'isConfigMocked' avoids nested calls to
	// Mock to reset the config to a blank state. This way we have only one mock per test and test helpers can call
	// Mock.
	if t != nil {
		m.Lock()
		defer m.Unlock()
		if isSystemProbeConfigMocked {
			// The configuration is already mocked.
			return &MockConfig{SystemProbe}
		}

		isSystemProbeConfigMocked = true
		originalConfig := SystemProbe
		t.Cleanup(func() {
			m.Lock()
			defer m.Unlock()
			isSystemProbeConfigMocked = false
			SystemProbe = originalConfig
		})
	}

	// Configure Datadog global configuration
	SystemProbe = NewConfig("system-probe", "DD", strings.NewReplacer(".", "_"))
	// Configuration defaults
	pkgconfigsetup.InitSystemProbeConfig(SystemProbe)
	return &MockConfig{SystemProbe}
}
