// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config provides the MCP server configuration component.
package config

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-shared-components

// Component is the component type for MCP configuration.
type Component interface {
	// Get returns the MCP server configuration
	Get() *MCPConfig
}

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newConfig))
}
