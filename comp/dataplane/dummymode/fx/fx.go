// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package fx provides fx wiring for the ADP dummy mode component
package fx

import (
	dummymode "github.com/DataDog/datadog-agent/comp/dataplane/dummymode/def"
	dummymodeimpl "github.com/DataDog/datadog-agent/comp/dataplane/dummymode/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"go.uber.org/fx"
)

// Module defines the fx options for this component
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			dummymodeimpl.NewComponent,
		),
		// Force the instantiation of the component, uses fx.Lifecycle for start/stop
		fx.Invoke(func(_ dummymode.Component) {}),
	)
}
