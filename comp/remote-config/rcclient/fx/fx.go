// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package fx provides the fx module for the rcclient component
package fx

import (
	rcclient "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/def"
	rcclientimpl "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"go.uber.org/fx"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(fxutil.ProvideComponentConstructor(rcclientimpl.NewRemoteConfigClient)),
		fxutil.ProvideOptional[rcclient.Component](),
	)
}

// NoneModule return a None optional type for rcclient.Component.
//
// This helper allows code that needs a disabled Optional type for rcclient to get it. The helper is split from
// the implementation to avoid linking with the dependencies from rcclient.
func NoneModule() fxutil.Module {
	return fxutil.Component(fx.Provide(func() option.Option[rcclient.Component] {
		return option.None[rcclient.Component]()
	}))
}
