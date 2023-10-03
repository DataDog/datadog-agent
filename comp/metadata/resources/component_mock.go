// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package resources

import (
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// MockParams defines the parameter for the mock resources metadata providers.
// It is designed to be used with `fx.Supply` and allows to set the return value for the resources mock.
//
//	fx.Supply(resourcesComponent.MockParams{Data: someData})
type MockParams struct {
	// Overrides is a parameter used to override values of the config
	Data map[string]interface{}
}

// Mock implements mock-specific methods for the resources component.
//
// Usage:
//
//	fxutil.Test[dependencies](
//	   t,
//	   resources.MockModule,
//	   fx.Replace(resources.MockParams{Data: someData}),
//	)
type Mock interface {
	Component
}

// MockModule defines the fx options for the mock component.
var MockModule = fxutil.Component(
	fx.Provide(newMock),
	fx.Supply(MockParams{}),
)
