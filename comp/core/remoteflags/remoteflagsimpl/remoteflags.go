// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package remoteflagsimpl provides the implementation for the Remote Flags component.
package remoteflagsimpl

import (
	comp "github.com/DataDog/datadog-agent/comp/core/remoteflags"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/remoteflags"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"go.uber.org/fx"
)

// Module defines the fx options for the Remote Flags component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newRemoteFlags))
}

type dependencies struct {
	fx.In

	// Subscribers is the list of components that subscribe to remote flags.
	// They are automatically collected via fx groups.
	Subscribers []remoteflags.RemoteFlagSubscriber `group:"remoteFlagSubscriber"`
}

type provides struct {
	fx.Out

	Comp       option.Option[comp.Component]
	RCListener types.ListenerProvider
}

type remoteFlagsComponent struct {
	client *remoteflags.Client
}

func newRemoteFlags(deps dependencies) provides {
	client := remoteflags.NewClient()
	component := &remoteFlagsComponent{
		client: client,
	}

	log.Info("Starting Remote Flags component")

	// Register all subscribers collected via fx groups
	for _, subscriber := range deps.Subscribers {
		for _, handler := range subscriber.Handlers() {
			if err := client.SubscribeWithHandler(handler); err != nil {
				log.Errorf("Failed to register remote flag handler for flag %s: %v", handler.FlagName(), err)
			} else {
				log.Debugf("Registered remote flag handler for flag %s", handler.FlagName())
			}
		}
	}

	// Use the RCListener pattern to automatically subscribe via FX dependency injection
	var rcListener types.ListenerProvider
	rcListener.ListenerProvider = types.RCListener{
		data.ProductAgentFlags: client.OnUpdate,
	}

	return provides{
		Comp:       option.New[comp.Component](component),
		RCListener: rcListener,
	}
}

// GetClient returns the remote flags client for subscribing to feature flags.
func (c *remoteFlagsComponent) GetClient() *remoteflags.Client {
	return c.client
}
