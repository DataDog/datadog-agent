// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package fx provides fx module for the forwarder component
package fx

import (
	"go.uber.org/fx"

	def "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/def"
	defaultforwarderimpl "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module(params def.Params) fxutil.Module {
	return fxutil.Component(
		fx.Provide(defaultforwarderimpl.NewForwarder),
		fx.Supply(params),
	)
}

// ModulWithOptionTMP defines the fx options for this component with an option.
// This is a temporary function to until configsync is cleanup.
func ModulWithOptionTMP(option fx.Option) fxutil.Module {
	return fxutil.Component(
		fx.Provide(defaultforwarderimpl.NewForwarder),
		option,
	)
}

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(defaultforwarderimpl.NewMockForwarder),
	)
}
