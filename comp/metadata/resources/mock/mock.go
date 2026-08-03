// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the resources metadata component
package mock

import (
	"testing"

	"go.uber.org/fx"

	resources "github.com/DataDog/datadog-agent/comp/metadata/resources/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// MockParams defines the parameter for the mock resources metadata providers.
// It is designed to be used with `fx.Supply` and allows to set the return value for the resources mock.
//
//	fx.Supply(resourcesmock.MockParams{Data: someData})
type MockParams struct {
	// Data is a parameter used to set the return value for the resources mock
	Data map[string]interface{}
}

// Mock implements mock-specific methods for the resources component.
//
// Usage:
//
//	fxutil.Test[dependencies](
//	   t,
//	   resourcesmock.MockModule(),
//	   fx.Replace(resourcesmock.MockParams{Data: someData}),
//	)
type Mock interface {
	resources.Component
}

type mockDependencies struct {
	fx.In

	Params MockParams
}

type mock struct {
	data map[string]interface{}
}

func (m *mock) Get() map[string]interface{} {
	return m.data
}

func newMock(deps mockDependencies, _ testing.TB) resources.Component {
	return &mock{
		data: deps.Params.Data,
	}
}

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMock),
		fx.Supply(MockParams{}))
}
