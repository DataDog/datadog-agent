// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package types provides fx wrappers for the Remote Flags component.
// It is kept separate from the component definition so that consumers
// who do not use fx (e.g. OTel) are not forced to depend on it.
package types

import (
	"github.com/DataDog/datadog-agent/pkg/remoteflags"
	"go.uber.org/fx"
)

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
