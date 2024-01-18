// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventoryagentimpl

import (
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"

	iainterface "github.com/DataDog/datadog-agent/comp/metadata/inventoryagent"
)

// MockModule defines the fx options for the mock component.
//
// Usage:
//
//	fxutil.Test[dependencies](
//	   t,
//	   inventoryagent.MockModule(),
//	)
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMock))
}

type inventoryagentMock struct{}

func newMock() iainterface.Component {
	return &inventoryagentMock{}
}

func (m *inventoryagentMock) Set(string, interface{}) {}

func (m *inventoryagentMock) GetAsJSON() ([]byte, error) {
	return []byte("{}"), nil
}

func (m *inventoryagentMock) Get() map[string]interface{} {
	return nil
}

func (m *inventoryagentMock) Refresh() {}
