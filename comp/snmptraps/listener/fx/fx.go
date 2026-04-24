// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package fx provides the fx module for the listener component.
package fx

import (
	"go.uber.org/fx"

	listener "github.com/DataDog/datadog-agent/comp/snmptraps/listener/def"
	listenerimpl "github.com/DataDog/datadog-agent/comp/snmptraps/listener/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(listenerimpl.NewComponent),
		fxutil.ProvideOptional[listener.Component](),
	)
}

// MockModule provides a mock listener for testing.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(listenerimpl.NewMockListener),
	)
}
