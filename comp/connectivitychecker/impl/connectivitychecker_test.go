// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package connectivitycheckerimpl

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

// Mock minimal de inventoryagent.Component
// (Set et Get, sans gestion de concurrence pour un test simple)
type mockInventoryAgent struct {
	data map[string]interface{}
}

func (m *mockInventoryAgent) Set(name string, value interface{}) {
	m.data[name] = value
}
func (m *mockInventoryAgent) Get() map[string]interface{} {
	return m.data
}

// Mock simple pour le lifecycle
type mockLifecycle struct {
	startHook compdef.Hook
	stopHook  compdef.Hook
}

func newMockLifecycle() *mockLifecycle {
	return &mockLifecycle{}
}

func (m *mockLifecycle) Append(hook compdef.Hook) {
	if hook.OnStart != nil {
		m.startHook = hook
	}
	if hook.OnStop != nil {
		m.stopHook = hook
	}
}

func (m *mockLifecycle) Start(ctx context.Context) error {
	if m.startHook.OnStart != nil {
		return m.startHook.OnStart(ctx)
	}
	return nil
}

func (m *mockLifecycle) Stop(ctx context.Context) error {
	if m.stopHook.OnStop != nil {
		return m.stopHook.OnStop(ctx)
	}
	return nil
}

func TestConnectivityInitialDelay(t *testing.T) {
	mockConfig := configmock.New(t)
	mockInventory := &mockInventoryAgent{data: make(map[string]interface{})}
	lifecycle := newMockLifecycle()

	// Configure the component
	mockConfig.SetWithoutSource("api_key", "test-key")
	mockConfig.SetWithoutSource("site", "datadoghq.com")

	reqs := Requires{
		Lifecycle:      lifecycle,
		Log:            logmock.New(t),
		Config:         mockConfig,
		InventoryAgent: mockInventory,
	}

	provides, err := NewComponent(reqs)
	require.NoError(t, err)
	assert.NotNil(t, provides.Comp)

	// Start the component
	ctx := context.Background()
	err = lifecycle.Start(ctx)
	defer lifecycle.Stop(ctx)
	require.NoError(t, err)

	// Wait for a short time (less than the initial delay)
	// The initial delay is 1 minute, so we wait 50ms to ensure we're well before it
	time.Sleep(50 * time.Millisecond)

	// Check that no data was set in the inventory (no collect should have run yet)
	initialData := mockInventory.Get()
	assert.Empty(t, initialData, "No collect operations should run before the initial delay")
}
