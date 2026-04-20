// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package remoteflagsimpl provides the implementation for the Remote Flags component.
package remoteflagsimpl

import (
	"context"

	comp "github.com/DataDog/datadog-agent/comp/core/remoteflags/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/remoteflags"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Requires defines the dependencies for the Remote Flags component.
type Requires struct {
	Lc compdef.Lifecycle

	// Subscribers is the list of components that subscribe to remote flags.
	// They are automatically collected via fx groups.
	Subscribers []remoteflags.RemoteFlagSubscriber `group:"remoteFlagSubscriber"`
}

// Provides defines the output of the Remote Flags component.
type Provides struct {
	Comp comp.Component
}

type remoteFlagsComponent struct {
	client *remoteflags.Client
}

// NewComponent creates a new Remote Flags component.
func NewComponent(deps Requires) Provides {
	client := remoteflags.NewClient()
	component := &remoteFlagsComponent{
		client: client,
	}

	log.Debug("Starting Remote Flags component")

	// Register all subscribers collected via fx groups
	for _, subscriber := range deps.Subscribers {
		for _, handler := range subscriber.Handlers() {
			if err := client.SubscribeWithHandler(handler); err != nil {
				log.Errorf("Remote flag %s: registration failed: %v", handler.FlagName(), err)
			}
		}
	}

	deps.Lc.Append(compdef.Hook{
		OnStop: func(_ context.Context) error {
			client.Stop()
			return nil
		},
	})

	return Provides{
		Comp: component,
	}
}

// GetClient returns the remote flags client for subscribing to feature flags.
func (c *remoteFlagsComponent) GetClient() *remoteflags.Client {
	return c.client
}
