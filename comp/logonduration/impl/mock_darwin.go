// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin && test

package logondurationimpl

import (
	"context"
	"sync"

	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/logonduration"
)

// mockSysProbeClient implements sysProbeClient for testing.
// This mock provides a testable implementation of the System Probe client
// interface, allowing tests to control the behavior of logon duration
// collection without requiring a real System Probe connection.
// It uses a mutex to protect concurrent access to mock state.
type mockSysProbeClient struct {
	mock.Mock
	mu sync.Mutex
}

// GetLoginTimestamps implements sysProbeClient.GetLoginTimestamps for testing.
// This method allows tests to specify expected calls and return values
// for login timestamp collection, enabling comprehensive test coverage
// of the logon duration component.
func (m *mockSysProbeClient) GetLoginTimestamps(ctx context.Context) (logonduration.LoginTimestamps, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	args := m.Called(ctx)
	return args.Get(0).(logonduration.LoginTimestamps), args.Error(1)
}

// GetCallCount returns the number of times GetLoginTimestamps was called.
// This method provides thread-safe access to check if the mock was called,
// avoiding race conditions when checking call state from test goroutines.
func (m *mockSysProbeClient) GetCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.Mock.Calls)
}
