// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fx provides the fx module for the workloadmeta component.
package fx

import (
	"go.uber.org/fx"

	wmdef "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// team: container-platform

// Module defines the fx options for this component.
func Module(params wmdef.Params) fxutil.Module {
	return module(fx.Supply(params))
}

// ModuleWithProvider defines the fx options for this component using a provider to get the parameter.
// T is the type of a component, typically component.Config.
func ModuleWithProvider[T any](paramsProvider func(T) wmdef.Params) fxutil.Module {
	return module(fx.Provide(paramsProvider))
}

func module(options ...fx.Option) fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			workloadmeta.NewWorkloadMeta,
		),
		fx.Provide(func(wmeta wmdef.Component) optional.Option[wmdef.Component] {
			return optional.NewOption(wmeta)
		}),
		fx.Options(options...),
	)
}
