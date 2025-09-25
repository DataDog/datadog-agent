// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package fx provides the fx module for the syntheticstestscheduler component
package fx

// team: synthetics-executing

import (
	syntheticstestscheduler "github.com/DataDog/datadog-agent/comp/syntheticstestscheduler/def"
	syntheticstestschedulerimpl "github.com/DataDog/datadog-agent/comp/syntheticstestscheduler/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// Module defines the fx options for this component
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			syntheticstestschedulerimpl.NewComponent,
		),
		fxutil.ProvideOptional[syntheticstestscheduler.Component](),
		// syntheticstestscheduler is a component with no public method, therefore nobody depends on it and FX only instantiates
		// components when they're needed. Adding a dummy function that takes our Component as a parameter force
		// the instantiation of syntheticstestscheduler. This means that simply using 'syntheticstestscheduler.Module()' will run our
		// component (which is the expected behavior).
		//
		// This prevents silent corner case where including 'syntheticstestscheduler' in the main function would not actually
		// instantiate it. This also remove the need for every main using syntheticstestscheduler to add the line bellow.
		fx.Invoke(func(_ syntheticstestscheduler.Component) {}),
	)
}
