// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fx defines the fx options for the observer component.
package fx

import (
	"go.uber.org/fx"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
	observerimpl "github.com/DataDog/datadog-agent/comp/observer/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for the observer component.
func Module() fxutil.Module {
	return fxutil.Component(
		// Provide default (empty) config - values come from datadog.yaml via pkgconfigsetup
		fx.Supply(observerimpl.AgentInternalLogTapConfig{}),
		fxutil.ProvideComponentConstructor(
			observerimpl.NewComponent,
		),
		fxutil.ProvideOptional[observerdef.Component](),
	)
}
