// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package fx provides the fx module for the config component.
package fx

import (
	"go.uber.org/fx"

	config "github.com/DataDog/datadog-agent/comp/snmptraps/config/def"
	configimpl "github.com/DataDog/datadog-agent/comp/snmptraps/config/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(configimpl.NewComponent),
		fxutil.ProvideOptional[config.Component](),
	)
}

// MockModule provides the default config for testing. Tests can override it
// with fx.Replace(&config.TrapsConfig{...}) to supply custom configuration.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(configimpl.NewMockConfig),
		fx.Supply(&config.TrapsConfig{Enabled: true}),
	)
}
