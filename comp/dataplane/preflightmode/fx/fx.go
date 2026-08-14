// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package fx provides fx wiring for the ADP preflight mode component
package fx

import (
	preflightmode "github.com/DataDog/datadog-agent/comp/dataplane/preflightmode/def"
	preflightmodeimpl "github.com/DataDog/datadog-agent/comp/dataplane/preflightmode/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"go.uber.org/fx"
)

// Module defines the fx options for this component
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			preflightmodeimpl.NewComponent,
		),
		// Force the instantiation of the component, uses fx.Lifecycle for start/stop
		fx.Invoke(func(_ preflightmode.Component) {}),
	)
}
