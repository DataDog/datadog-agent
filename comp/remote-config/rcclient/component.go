// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package rcclient

import (
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// team: remote-config

// Component is the component type.
type Component interface {
	// TODO: (components) Start the remote config client to listen to AGENT_TASK configurations
	// Once the remote config client is refactored and can push updates directly to the listeners,
	// we can remove this.
	Listen(clientName string, products []data.Product) error
}

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newRemoteConfigClient),
)
