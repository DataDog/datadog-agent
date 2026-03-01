// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config implements a component to handle trace-agent configuration.
// Deprecated: import from comp/trace/config/def, comp/trace/config/fx,
// or comp/trace/config/mock instead.
package config

import (
	traceconfigdef "github.com/DataDog/datadog-agent/comp/trace/config/def"
	traceconfigimpl "github.com/DataDog/datadog-agent/comp/trace/config/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"go.uber.org/fx"
)

// team: agent-apm

// Component is the component type.
// Deprecated: Use comp/trace/config/def.Component directly.
type Component = traceconfigdef.Component

// Params defines the parameters for the config component.
// Deprecated: Use comp/trace/config/def.Params directly.
type Params = traceconfigdef.Params

// Module defines the fx options for this component.
// Deprecated: Use comp/trace/config/fx.Module() instead.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			traceconfigimpl.NewComponent,
		),
		fx.Supply(Params{
			FailIfAPIKeyMissing: true,
		}),
	)
}

// LoadConfigFile is re-exported for callers that don't use components.
// Deprecated: Use comp/trace/config/impl.LoadConfigFile directly.
var LoadConfigFile = traceconfigimpl.LoadConfigFile
