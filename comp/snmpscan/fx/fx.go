// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package fx provides the fx module for the snmpscan component
package fx

import (
	"go.uber.org/fx"

	snmpscan "github.com/DataDog/datadog-agent/comp/snmpscan/def"
	snmpscanimpl "github.com/DataDog/datadog-agent/comp/snmpscan/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			snmpscanimpl.NewComponent,
		),
		fxutil.ProvideOptional[snmpscan.Component](),

		// snmpscan is a component with no public method, therefore nobody depends on it and FX only instantiates
		// components when they're needed. Adding a dummy function that takes our Component as a parameter forces
		// the instantiation of snmpscan. This means that simply using 'snmpscan.Module()' will run our
		// component (which is the expected behavior).
		//
		// This prevents silent corner case where including 'snmpscan' in the main function would not actually
		// instantiate it. This also removes the need for every main using snmpscan to add the line below.
		fx.Invoke(func(_ snmpscan.Component) {}),
	)
}
