// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fx provides the fx module for the software inventory component
package fx

import (
	"github.com/DataDog/datadog-agent/comp/metadata/softwareinventory/def"
	"github.com/DataDog/datadog-agent/comp/metadata/softwareinventory/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// Module defines the fx options for this component
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			softwareinventoryimpl.New,
		),
		fxutil.ProvideOptional[softwareinventory.Component](),

		// software inventory is a component that nobody depends on it and FX only instantiates
		// components when they're needed. Adding a dummy function that takes our Component as a parameter force
		// the instantiation of software inventory. This means that simply using 'softwareinventory.Module()' will run our
		// component (which is the expected behavior).
		fx.Invoke(func(_ softwareinventory.Component) {}),
	)
}
