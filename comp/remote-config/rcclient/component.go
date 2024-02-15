// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package rcclient //nolint:revive // TODO(RC) Fix revive linter

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: remote-config

// Component is the component type.
type Component interface {
	// TODO: (components) Subscribe to AGENT_CONFIG configurations and start the remote config client
	// Once the remote config client is refactored and can push updates directly to the listeners,
	// we can remove this.
	Start(agentName string) error
	// SubscribeAgentTask subscribe the remote-config client to AGENT_TASK
	SubscribeAgentTask()
	// SubscribeApmTracing subscribes the remote-config client to APM_TRACING
	SubscribeApmTracing()
	// Subscribe is the generic way to start listening to a specific product update
	// Component can also automatically subscribe to updates by returning a `ListenerProvider` struct
	Subscribe(product data.Product, fn func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)))
}

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newRemoteConfigClient))
}
