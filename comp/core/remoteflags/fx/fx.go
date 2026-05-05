// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package fx provides the fx module for the remoteflags component.
package fx

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	remoteflags "github.com/DataDog/datadog-agent/comp/core/remoteflags/def"
	remoteflagsimpl "github.com/DataDog/datadog-agent/comp/core/remoteflags/impl"
	noopimpl "github.com/DataDog/datadog-agent/comp/core/remoteflags/impl-noop"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	pkgremoteflags "github.com/DataDog/datadog-agent/pkg/remoteflags"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// Requires defines the dependencies needed to choose between the real and the
// no-op Remote Flags component.
type Requires struct {
	Lc          compdef.Lifecycle
	Config      config.Component
	Subscribers []pkgremoteflags.RemoteFlagSubscriber `group:"remoteFlagSubscriber"`
}

// Provides defines the output of the Remote Flags fx module.
type Provides struct {
	Comp remoteflags.Component
}

// Module defines the fx options for the Remote Flags component.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(newComponent),
		fx.Provide(newRCListener),
	)
}

// newComponent dispatches between the real implementation and the no-op
// implementation based on the `remote_flags.enabled` config field. A
// constructor is always registered in the fx graph so consumers can depend on
// remoteflags.Component without crashing fx when the feature is disabled.
func newComponent(deps Requires) Provides {
	if !deps.Config.GetBool("remote_flags.enabled") {
		return Provides{Comp: noopimpl.NewComponent()}
	}
	real := remoteflagsimpl.NewComponent(remoteflagsimpl.Requires{
		Lc:          deps.Lc,
		Subscribers: deps.Subscribers,
	})
	return Provides{Comp: real.Comp}
}

func newRCListener(cfg config.Component, comp remoteflags.Component) types.ListenerProvider {
	if !cfg.GetBool("remote_flags.enabled") {
		return types.ListenerProvider{}
	}
	var rcListener types.ListenerProvider
	rcListener.ListenerProvider = types.RCListener{
		data.ProductAgentFlags: comp.GetClient().OnUpdate,
	}
	return rcListener
}
