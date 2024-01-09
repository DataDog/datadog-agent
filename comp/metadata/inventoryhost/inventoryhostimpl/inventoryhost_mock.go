// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventoryhostimpl

import (
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryhost"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// MockModule defines the fx options for the mock component.
//
// Usage:
//
//	fxutil.Test[dependencies](
//	   t,
//	   inventoryhost.MockModule(),
//	)
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMock))
}

type inventoryhostMock struct{}

func newMock() inventoryhost.Component {
	return &inventoryhostMock{}
}

func (m *inventoryhostMock) GetAsJSON() ([]byte, error) {
	return []byte("{}"), nil
}

func (m *inventoryhostMock) Refresh() {}
