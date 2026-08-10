// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the workloadbalancing component
package mock

import (
	"maps"
	"sync"

	"go.uber.org/fx"

	workloadbalancing "github.com/DataDog/datadog-agent/comp/workloadbalancing/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type mockWorkloadBalancing struct {
	mu      sync.RWMutex
	enabled bool
	groups  map[string]workloadbalancing.State
}

func (m *mockWorkloadBalancing) Enabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.enabled
}

func (m *mockWorkloadBalancing) GetGroupState(groupID string) workloadbalancing.State {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, ok := m.groups[groupID]
	if !ok {
		return workloadbalancing.Unmanaged
	}
	return state
}

func (m *mockWorkloadBalancing) GetGroupStates() map[string]workloadbalancing.State {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return maps.Clone(m.groups)
}

func (m *mockWorkloadBalancing) IsGroupActive(groupID string) bool {
	return m.GetGroupState(groupID) != workloadbalancing.Standby
}

func (m *mockWorkloadBalancing) SetEnabled(enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.enabled = enabled
}

func (m *mockWorkloadBalancing) SetGroupState(groupID string, state workloadbalancing.State) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.groups[groupID] = state
}

// Component is the component type.
type Component interface {
	workloadbalancing.Component

	SetEnabled(bool)
	SetGroupState(string, workloadbalancing.State)
}

// NewMock returns a new Mock
func NewMock() Component {
	return &mockWorkloadBalancing{
		enabled: false,
		groups:  make(map[string]workloadbalancing.State),
	}
}

// Module defines the fx options for the mock component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(func() workloadbalancing.Component { return NewMock() }),
	)
}
