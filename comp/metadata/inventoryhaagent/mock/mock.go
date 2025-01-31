// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package mock provides a mock for the inventoryhaagent component
package mock

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	inventoryhaagent "github.com/DataDog/datadog-agent/comp/metadata/inventoryhaagent/def"
)

// Module defines the fx options for the mock component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMock))
}

type inventoryhaagentMock struct{}

func newMock() inventoryhaagent.Component {
	return &inventoryhaagentMock{}
}

func (m *inventoryhaagentMock) GetAsJSON() ([]byte, error) {
	return []byte("{}"), nil
}

func (m *inventoryhaagentMock) Get() map[string]interface{} {
	return nil
}

func (m *inventoryhaagentMock) Refresh() {}
