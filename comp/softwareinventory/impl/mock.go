// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package softwareinventoryimpl

import (
	"context"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	"github.com/DataDog/datadog-agent/pkg/inventory/software"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/stretchr/testify/mock"
)

// mockSysProbeClient implements mockSysProbeClient for testing.
// This mock provides a testable implementation of the System Probe client
// interface, allowing tests to control the behavior of software inventory
// collection without requiring a real System Probe connection.
// It uses a mutex to protect concurrent access to mock state.
type mockSysProbeClient struct {
	mock.Mock
	mu sync.Mutex
}

// GetCallCount returns the number of times GetCheck was called.
// This method provides thread-safe access to check if the mock was called,
// avoiding race conditions when checking call state from test goroutines.
func (m *mockSysProbeClient) GetCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.Mock.Calls)
}

// AssertNumberOfCalls wraps the mock.Mock AssertNumberOfCalls with thread-safety.
// This method provides thread-safe access to assert call counts.
func (m *mockSysProbeClient) AssertNumberOfCalls(t mock.TestingT, methodName string, expectedCalls int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Mock.AssertNumberOfCalls(t, methodName, expectedCalls)
}

// GetCheck implements the mockSysProbeClient interface for testing.
// This method allows tests to specify expected calls and return values
// for software inventory collection, enabling comprehensive test coverage
// of the inventory software component.
func (m *mockSysProbeClient) GetCheck(module types.ModuleName) ([]software.Entry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	args := m.Called(module)
	argValue := args.Get(0)
	if argValue == nil {
		return nil, args.Error(1)
	}
	return argValue.([]software.Entry), args.Error(1)
}

// mockHostname implements hostnameinterface.Component for testing.
// This mock provides a consistent hostname for testing purposes,
// ensuring that tests have predictable hostname values without
// depending on the actual system hostname.
type mockHostname struct{}

// GetWithProvider returns test hostname data with provider information.
// This method provides a complete hostname data structure for testing,
// including both the hostname string and the provider information.
func (m *mockHostname) GetWithProvider(_ context.Context) (hostnameinterface.Data, error) {
	return hostnameinterface.Data{
		Hostname: "test-hostname",
		Provider: "test-provider",
	}, nil
}

// GetSafe returns a safe hostname string for testing.
// This method provides a fallback hostname value that can be used
// when the primary hostname retrieval fails or is not available.
func (m *mockHostname) GetSafe(_ context.Context) string {
	return "test-hostname"
}

// Get returns the test hostname with error handling.
// This method provides the standard hostname retrieval interface
// for testing, returning a consistent test hostname value.
func (m *mockHostname) Get(_ context.Context) (string, error) {
	return "test-hostname", nil
}
