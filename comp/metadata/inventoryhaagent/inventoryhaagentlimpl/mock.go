// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventoryhaagentlimpl

import (
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"

	iointerface "github.com/DataDog/datadog-agent/comp/metadata/inventoryhaagent"
)

// MockModule defines the fx options for the mock component.
//
// Usage:
//
//	fxutil.Test[dependencies](
//	   t,
//	   inventoryhaagentimpl.MockModule(),
//	)
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMock))
}

type inventoryhaagentMock struct{}

func newMock() iointerface.Component {
	return &inventoryhaagentMock{}
}

func (m *inventoryhaagentMock) GetAsJSON() ([]byte, error) {
	return []byte("{}"), nil
}

func (m *inventoryhaagentMock) Get() map[string]interface{} {
	return nil
}

func (m *inventoryhaagentMock) Refresh() {}
