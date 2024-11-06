// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package server implements a component that runs the netflow server.
// When running, it listens for network traffic according to configured
// listeners and aggregates traffic data to send to the backend.
// It does not expose any public methods.
package server

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: ndm-integrations

// Component is the component type. It has no exposed methods.
type Component interface{}

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newServer))
}
