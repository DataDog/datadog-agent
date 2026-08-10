// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package fx provides the fx module for the privateactionrunner component
package fx

import (
	privateactionrunner "github.com/DataDog/datadog-agent/comp/privateactionrunner/def"
	privateactionrunnerimpl "github.com/DataDog/datadog-agent/comp/privateactionrunner/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

type shutdownerAdapter struct {
	shutdowner fx.Shutdowner
}

func (s shutdownerAdapter) Shutdown() error {
	return s.shutdowner.Shutdown()
}

func newShutdownerAdapter(shutdowner fx.Shutdowner) privateactionrunnerimpl.Shutdowner {
	return shutdownerAdapter{shutdowner: shutdowner}
}

// Module defines the fx options for this component
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newShutdownerAdapter),
		fxutil.ProvideComponentConstructor(
			privateactionrunnerimpl.NewComponent,
		),
		fxutil.ProvideOptional[privateactionrunner.Component](),
		// Force instantiation since no other component depends on privateactionrunner
		fx.Invoke(func(_ privateactionrunner.Component) {}),
	)
}

// ExecutorModule defines the fx options for the on-demand executor mode.
func ExecutorModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newShutdownerAdapter),
		fxutil.ProvideComponentConstructor(
			privateactionrunnerimpl.NewExecutorComponent,
		),
		fxutil.ProvideOptional[privateactionrunner.Component](),
		// Force instantiation since no other component depends on privateactionrunner
		fx.Invoke(func(_ privateactionrunner.Component) {}),
	)
}
