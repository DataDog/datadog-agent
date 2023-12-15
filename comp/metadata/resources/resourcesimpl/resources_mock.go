// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package resourcesimpl

import (
	"testing"

	resources "github.com/DataDog/datadog-agent/comp/metadata/resources"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMock),
		fx.Supply(resources.MockParams{}))
}

type mockDependencies struct {
	fx.In

	Params resources.MockParams
}

type mock struct {
	data map[string]interface{}
}

func (m *mock) Get() map[string]interface{} {
	return m.data
}

func newMock(deps mockDependencies, t testing.TB) resources.Component { //nolint:revive // TODO fix revive unused-parameter
	return &mock{
		data: deps.Params.Data,
	}
}
