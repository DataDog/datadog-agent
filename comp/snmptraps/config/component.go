// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package config implements the configuration type for the traps server and
// a component that provides it.
package config

import (
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// team: network-device-monitoring

// Component is the component type.
type Component interface {
	Get() *TrapsConfig
}

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newService),
)

// MockModule provides the default config, and allows tests to override it by
// providing `fx.Replace(&TrapsConfig{...})`; a value replaced this way will
// have default values set sensibly if they aren't provided.
var MockModule = fxutil.Component(
	fx.Provide(newMockConfig),
	fx.Supply(&TrapsConfig{}),
)
