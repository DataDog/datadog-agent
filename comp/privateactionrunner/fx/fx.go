// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package fx provides the fx module for the privateactionrunner component
package fx

import (
	"go.uber.org/fx"

	config "github.com/DataDog/datadog-agent/comp/core/config"
	privateactionrunner "github.com/DataDog/datadog-agent/comp/privateactionrunner/def"
	privateactionrunnerimpl "github.com/DataDog/datadog-agent/comp/privateactionrunner/impl"
	rctypes "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
	taskverifier "github.com/DataDog/datadog-agent/pkg/privateactionrunner/task-verifier"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newKeysManager),
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
		fx.Provide(newKeysManager),
		fxutil.ProvideComponentConstructor(
			privateactionrunnerimpl.NewExecutorComponent,
		),
		fxutil.ProvideOptional[privateactionrunner.Component](),
		// Force instantiation since no other component depends on privateactionrunner
		fx.Invoke(func(_ privateactionrunner.Component) {}),
	)
}

type keysManagerProvides struct {
	fx.Out

	Manager  taskverifier.KeysManager
	Listener rctypes.RCListener `group:"rCListener"`
}

func newKeysManager(cfg config.Component) keysManagerProvides {
	manager, callback := taskverifier.NewKeyManagerWithCallback()
	var listener rctypes.RCListener
	if callback != nil && cfg.GetBool(privateactionrunner.PAREnabled) {
		listener = rctypes.RCListener{
			state.ProductActionPlatformRunnerKeys: callback,
		}
	}
	return keysManagerProvides{Manager: manager, Listener: listener}
}
