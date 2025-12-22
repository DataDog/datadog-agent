// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package server implements a component that runs the MCP (Model Context Protocol) server.
// When running, it listens on stdio for MCP protocol messages and responds accordingly.
// It does not expose any public methods as it operates autonomously.
package server

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-shared-components

// Component is the component type. It has no exposed methods.
type Component interface{}

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMCPServer))
}
