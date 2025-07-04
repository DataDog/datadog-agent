// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package mock

import (
	inventoryagent "github.com/DataDog/datadog-agent/comp/metadata/inventoryagent/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// Mock implements mock-specific methods for the inventoryagent component.
type Mock interface {
	inventoryagent.Component
}

// Module defines the fx options for the mock component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(NewMock),
	)
}

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	return Module()
}

// NewMock creates a new mock inventoryagent component
func NewMock() inventoryagent.Component {
	return &mockInventoryAgent{
		data: make(map[string]interface{}),
	}
}

type mockInventoryAgent struct {
	data map[string]interface{}
}

func (m *mockInventoryAgent) Set(name string, value interface{}) {
	m.data[name] = value
}

func (m *mockInventoryAgent) Get() map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range m.data {
		result[k] = v
	}
	return result
}
