// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package logsagenthealthfx provides the FX module for the logs agent health checker sub-component.
package logsagenthealthfx

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logsagenthealth "github.com/DataDog/datadog-agent/comp/core/health-platform/logs-agent-health/def"
	logsagenthealthimpl "github.com/DataDog/datadog-agent/comp/core/health-platform/logs-agent-health/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module() fx.Option {
	return fx.Module("logs-agent-health-checker",
		fxutil.ProvideComponentConstructor(newLogsAgentHealthChecker),
	)
}

// Dependencies lists the dependencies for the logs agent health checker
// Note: Keep this a plain struct; ProvideComponentConstructor will wrap with fx.In.
type Dependencies struct {
	Config config.Component
}

// newLogsAgentHealthChecker creates a new logs agent health checker component.
func newLogsAgentHealthChecker(deps Dependencies) logsagenthealth.Component {
	return logsagenthealthimpl.NewComponent(logsagenthealthimpl.Dependencies{
		Config: deps.Config,
	})
}
