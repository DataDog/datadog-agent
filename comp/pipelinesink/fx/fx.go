// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fx provides the fx module for the pipelinesink component.
package fx

import (
	pipelinesink "github.com/DataDog/datadog-agent/comp/pipelinesink/def"
	pipelinesinkimpl "github.com/DataDog/datadog-agent/comp/pipelinesink/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// Module defines the fx options for the pipelinesink component.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			pipelinesinkimpl.NewComponent,
		),
		fxutil.ProvideOptional[pipelinesink.Component](),
		// Force instantiation: pipelinesink is self-contained and not consumed
		// by any other component, so we must invoke it explicitly.
		fx.Invoke(func(_ pipelinesink.Component) {}),
	)
}
