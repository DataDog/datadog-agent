// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package fx provides fx options for the payload modifier component
package fx

import (
	"go.uber.org/fx"

	payloadmodifier "github.com/DataDog/datadog-agent/comp/trace/payload-modifier/def"
	payloadmodifierimpl "github.com/DataDog/datadog-agent/comp/trace/payload-modifier/impl"
	pkgagent "github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for the payload modifier component
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			payloadmodifierimpl.NewComponent,
		),
		fx.Provide(func(comp payloadmodifier.Component) pkgagent.TracerPayloadModifier {
			return comp
		}),
	)
}

// NilModule provides a nil TracerPayloadModifier for contexts that need to satisfy
// the dependency but don't require payload modification functionality.
func NilModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(func() pkgagent.TracerPayloadModifier { return nil }))
}
