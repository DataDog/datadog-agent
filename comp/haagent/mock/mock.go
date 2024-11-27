// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the haagent component
package mock

import (
	"go.uber.org/fx"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	haagent "github.com/DataDog/datadog-agent/comp/haagent/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type mockHaAgent struct {
	Logger log.Component

	group   string
	enabled bool
}

func (m *mockHaAgent) GetGroup() string {
	return m.group
}

func (m *mockHaAgent) Enabled() bool {
	return m.enabled
}

func (m *mockHaAgent) SetLeader(_ string) {
}

func (m *mockHaAgent) IsLeader() bool { return false }

func (m *mockHaAgent) SetGroup(group string) {
	m.group = group
}

func (m *mockHaAgent) SetEnabled(enabled bool) {
	m.enabled = enabled
}

// Component is the component type.
type Component interface {
	haagent.Component

	SetGroup(string)
	SetEnabled(bool)
}

// NewMockHaAgent returns a new Mock
func NewMockHaAgent() haagent.Component {
	return &mockHaAgent{
		enabled: false,
		group:   "group01",
	}
}

// Module defines the fx options for the mock component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(NewMockHaAgent),
	)
}
