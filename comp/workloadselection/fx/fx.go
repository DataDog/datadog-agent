// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package fx provides the fx module for the workloadselection component
package fx

import (
	uberfx "go.uber.org/fx"

	workloadselection "github.com/DataDog/datadog-agent/comp/workloadselection/def"
	workloadselectionimpl "github.com/DataDog/datadog-agent/comp/workloadselection/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			workloadselectionimpl.NewComponent,
		),
		fxutil.ProvideOptional[workloadselection.Component](),

		// workloadselection is a component with no public method, therefore nobody depends on it and FX only instantiates
		// components when they're needed. Adding a dummy function that takes our Component as a parameter forces
		// the instantiation of workloadselection. This means that simply using 'workloadselectionfx.Module()' will run our
		// component (which is the expected behavior).
		//
		// This prevents silent corner case where including 'workloadselection' in the main function would not actually
		// instantiate it. This also removes the need for every main using workloadselection to add the line below.
		uberfx.Invoke(func(_ workloadselection.Component) {}),
	)
}
