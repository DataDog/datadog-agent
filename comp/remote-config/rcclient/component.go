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
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// team: remote-config

// Component is the component type.
type Component interface {
	// SubscribeAgentTask subscribe the remote-config client to AGENT_TASK
	SubscribeAgentTask()
	// SubscribeApmTracing subscribes the remote-config client to APM_TRACING
	SubscribeApmTracing()
	// Subscribe is the generic way to start listening to a specific product update
	// Component can also automatically subscribe to updates by returning a `ListenerProvider` struct
	Subscribe(product data.Product, fn func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)))
}

// Params is the input parameter struct for the RC client Component.
type Params struct {
	AgentName     string
	AgentVersion  string
	IsSystemProbe bool
}

// NoneModule return a None optional type for rcclient.Component.
//
// This helper allows code that needs a disabled Optional type for rcclient to get it. The helper is split from
// the implementation to avoid linking with the dependencies from rcclient.
func NoneModule() fxutil.Module {
	return fxutil.Component(fx.Provide(func() optional.Option[Component] {
		return optional.NewNoneOption[Component]()
	}))
}
