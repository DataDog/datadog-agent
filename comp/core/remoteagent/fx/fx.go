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
func Module(params remoteagent.Params) fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			remoteagentimpl.NewComponent,
		),
		fx.Supply(params),
		fxutil.ProvideOptional[remoteagent.Component](),
	)
}
