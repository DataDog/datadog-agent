// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"net/http"

	"go.uber.org/fx"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	traceconfig "github.com/DataDog/datadog-agent/pkg/trace/config"
)

// team: agent-apm

type dependencies struct {
	fx.In
	Params Params
	Config coreconfig.Component
}

// cfg implements the Component.
type cfg struct {
	// this component is currently implementing a thin wrapper around pkg/trace/config,
	// and uses globals in that package.
	*traceconfig.AgentConfig

	// coreConfig relates to the main agent config component
	coreConfig coreconfig.Component

	// warnings are the warnings generated during setup
	warnings *pkgconfig.Warnings
}

func newConfig(deps dependencies) (Component, error) {
	panic("not called")
}

func (c *cfg) Warnings() *pkgconfig.Warnings {
	panic("not called")
}

func (c *cfg) Object() *traceconfig.AgentConfig {
	panic("not called")
}

// SetHandler returns handler for runtime configuration changes.
func (c *cfg) SetHandler() http.Handler {
	panic("not called")
}

// SetMaxMemCPU sets watchdog's max_memory and max_cpu_percent parameters.
// If the agent is containerized, max_memory and max_cpu_percent are disabled by default.
// Resource limits are better handled by container runtimes and orchestrators.
func (c *cfg) SetMaxMemCPU(isContainerized bool) {
	panic("not called")
}
