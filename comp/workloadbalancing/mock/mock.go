// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the workloadbalancing component
package mock

import (
	"go.uber.org/fx"

	workloadbalancing "github.com/DataDog/datadog-agent/comp/workloadbalancing/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type mockWorkloadBalancing struct {
	enabled     bool
	groupActive map[string]bool
}

func (m *mockWorkloadBalancing) IsGroupActive(groupID string) bool {
	if active, ok := m.groupActive[groupID]; ok {
		return active
	}
	return true
}

func (m *mockWorkloadBalancing) Enabled() bool {
	return m.enabled
}

// Component is the component type.
type Component interface {
	workloadbalancing.Component

	SetGroupActive(groupID string, active bool)
	SetEnabled(enabled bool)
}

func (m *mockWorkloadBalancing) SetGroupActive(groupID string, active bool) {
	m.groupActive[groupID] = active
}

func (m *mockWorkloadBalancing) SetEnabled(enabled bool) {
	m.enabled = enabled
}

// NewMockWorkloadBalancing returns a new Mock
func NewMockWorkloadBalancing() workloadbalancing.Component {
	return &mockWorkloadBalancing{
		enabled:     true,
		groupActive: make(map[string]bool),
	}
}

// Module defines the fx options for the mock component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(NewMockWorkloadBalancing),
	)
}
