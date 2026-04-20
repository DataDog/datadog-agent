// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package remoteflags provides the Remote Flags component for dynamic feature flag management.
package remoteflags

import (
	"github.com/DataDog/datadog-agent/pkg/remoteflags"
	"go.uber.org/fx"
)

// team: agent-metric-pipelines

// Component is the Remote Flags component interface.
type Component interface {
	// GetClient returns the remote flags client for subscribing to feature flags.
	GetClient() *remoteflags.Client
}

// RemoteFlagSubscriber is the fx wrapper for components that subscribe to remote flags.
// Components that want to subscribe to remote flags should return this from their
// fx.Provide function.
type RemoteFlagSubscriber struct {
	fx.Out

	Subscriber remoteflags.RemoteFlagSubscriber `group:"remoteFlagSubscriber"`
}

// NewRemoteFlagSubscriber creates a RemoteFlagSubscriber for fx registration.
// Pass a component that implements remoteflags.RemoteFlagSubscriber.
func NewRemoteFlagSubscriber(subscriber remoteflags.RemoteFlagSubscriber) RemoteFlagSubscriber {
	return RemoteFlagSubscriber{
		Subscriber: subscriber,
	}
}
