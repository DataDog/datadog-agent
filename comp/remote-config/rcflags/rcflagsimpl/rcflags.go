// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package rcflagsimpl provides the implementation for the Remote Flags component.
package rcflagsimpl

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcflags"
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

	RCClient option.Option[rcclient.Component]
	Lc       fx.Lifecycle
}

type remoteFlagsComponent struct {
	client   *remoteflags.Client
	rcClient option.Option[rcclient.Component]
}

func newRemoteFlags(deps dependencies) option.Option[rcflags.Component] {
	// If Remote Config client is not available, return None
	rcClientComp, ok := deps.RCClient.Get()
	if !ok {
		log.Info("Remote Config client not available, Remote Flags component will not be initialized")
		return option.None[rcflags.Component]()
	}

	client := remoteflags.NewClient()
	component := &remoteFlagsComponent{
		client:   client,
		rcClient: deps.RCClient,
	}

	// Register lifecycle hooks
	deps.Lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			log.Info("Starting Remote Flags component")
			// Subscribe to Remote Flags product
			rcClientComp.Subscribe(data.ProductRemoteFlags, client.OnUpdate)
			return nil
		},
		OnStop: func(_ context.Context) error {
			log.Info("Stopping Remote Flags component")
			return nil
		},
	})

	return option.New[rcflags.Component](component)
}

// GetClient returns the remote flags client for subscribing to feature flags.
func (c *remoteFlagsComponent) GetClient() *remoteflags.Client {
	return c.client
}
