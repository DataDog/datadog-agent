// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package fx provides the fx module for the dual tagger component
package fx

import (
	"go.uber.org/fx"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	dualimpl "github.com/DataDog/datadog-agent/comp/core/tagger/impl-dual"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component
func Module(dualParams tagger.DualParams, localParams tagger.Params, remoteParams tagger.RemoteParams) fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			dualimpl.NewComponent,
		),
		fx.Supply(localParams),
		fx.Supply(remoteParams),
		fx.Supply(dualParams),
		fxutil.ProvideOptional[tagger.Component](),
	)
}
