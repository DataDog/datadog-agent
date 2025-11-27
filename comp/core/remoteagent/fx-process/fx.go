// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package fx provides the fx module for the remoteagent component
package fx

import (
	"go.uber.org/fx"

	remoteagent "github.com/DataDog/datadog-agent/comp/core/remoteagent/def"
	remoteagentimpl "github.com/DataDog/datadog-agent/comp/core/remoteagent/impl-process"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			remoteagentimpl.NewComponent,
		),
		fxutil.ProvideOptional[remoteagent.Component](),

		// Since no other component depends on remoteagent, we add this dummy invocation to ensure it gets instantiated when the module is used.
		fx.Invoke(func(_ remoteagent.Component) {}),
	)
}
