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
	isSystemProbeConfigMocked = false
	m                         = sync.Mutex{}
)

// mockConfig should only be used in tests
type mockConfig struct {
	Config
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

// MockSystemProbe is creating and returning a mock system-probe Config
//
// This method is deprecated and will soon be removed. Use pkg/config/mock.NewSystemProbe instead.
func MockSystemProbe(t testing.TB) model.Config {
	// We only check isSystemProbeConfigMocked when registering a cleanup function. 'isSystemProbeConfigMocked'
	// avoids nested calls to Mock to reset the config to a blank state. This way we have only one mock per test and
	// test helpers can call Mock.
	if t != nil {
		m.Lock()
		defer m.Unlock()
		if isSystemProbeConfigMocked {
			// The configuration is already mocked.
			return &mockConfig{pkgconfigsetup.SystemProbe()}
		}

		isSystemProbeConfigMocked = true
		originalConfig := pkgconfigsetup.SystemProbe()
		t.Cleanup(func() {
			m.Lock()
			defer m.Unlock()
			isSystemProbeConfigMocked = false
			pkgconfigsetup.SetSystemProbe(originalConfig)
		})
	}

	// Configure Datadog global configuration
	pkgconfigsetup.SetSystemProbe(NewConfig("system-probe", "DD", strings.NewReplacer(".", "_")))
	// Configuration defaults
	pkgconfigsetup.InitSystemProbeConfig(pkgconfigsetup.SystemProbe())
	return &mockConfig{pkgconfigsetup.SystemProbe()}
}
