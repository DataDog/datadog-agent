// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"sync"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config/model"
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
	panic("not called")
}

// SetWithoutSource is used for setting configuration in tests
func (c *MockConfig) SetWithoutSource(key string, value interface{}) {
	panic("not called")
}

// Mock is creating and returning a mock config
func Mock(t testing.TB) *MockConfig {
	panic("not called")
}

// MockSystemProbe is creating and returning a mock system-probe config
func MockSystemProbe(t testing.TB) *MockConfig {
	panic("not called")
}
