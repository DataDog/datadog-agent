// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package fx provides the fx module for the fleetstatus component
package fx

import (
	uberfx "go.uber.org/fx"

	fleetstatus "github.com/DataDog/datadog-agent/comp/fleetstatus/def"
	fleetstatusimpl "github.com/DataDog/datadog-agent/comp/fleetstatus/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			fleetstatusimpl.NewComponent,
		),
		fxutil.ProvideOptional[fleetstatus.Component](),

		// fleetStatus is a component with no public method, therefore nobody depends on it and FX only instantiates
		// components when they're needed. Adding a dummy function that takes our Component as a parameter force
		// the instantiation of fleetStatus. This means that simply using 'fleetStatus.Module()' will run our
		// component (which is the expected behavior).
		//
		// This prevent silent corner case where including 'fleetStatus' in the main function would not actually
		// instantiate it. This also remove the need for every main using fleetStatus to add the line bellow.
		uberfx.Invoke(func(_ fleetstatus.Component) {}),
	)
}
