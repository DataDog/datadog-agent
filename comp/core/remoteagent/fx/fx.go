// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package fx provides the fx module for the remoteagent component
package fx

import (
	"go.uber.org/fx"

	remoteagent "github.com/DataDog/datadog-agent/comp/core/remoteagent/def"
	remoteagentimpl "github.com/DataDog/datadog-agent/comp/core/remoteagent/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			remoteagentimpl.NewComponent,
		),
		fxutil.ProvideOptional[remoteagent.Component](),
		// remoteagent is a component with no public method, therefore nobody depends on it and FX only instantiates
		// components when they're needed. Adding a dummy function that takes our Component as a parameter force
		// the instantiation of remoteagent. This means that simply using 'configsync.Module()' will run our
		// component (which is the expected behavior).
		//
		// This prevent silent corner case where including 'remoteagent' in the main function would not actually
		// instantiate it. This also remove the need for every main using remoteagent to add the line bellow.
		fx.Invoke(func(_ remoteagent.Component) {}),
	)
}
