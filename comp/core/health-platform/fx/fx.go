// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package fx provides the fx module for the health platform component
package fx

import (
	"go.uber.org/fx"

	healthplatform "github.com/DataDog/datadog-agent/comp/core/health-platform/def"
	healthplatformimpl "github.com/DataDog/datadog-agent/comp/core/health-platform/impl"
	logsagenthealth "github.com/DataDog/datadog-agent/comp/core/health-platform/logs-agent-health/def"
	logsagenthealthfx "github.com/DataDog/datadog-agent/comp/core/health-platform/logs-agent-health/fx"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Module defines the fx options for this component
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			healthplatformimpl.NewComponent,
		),
		fxutil.ProvideOptional[healthplatform.Component](),
		// Include the logs agent health sub-component
		logsagenthealthfx.Module(),
		// Automatically register the logs agent health sub-component with the health platform
		fx.Invoke(func(healthPlatform healthplatform.Component, logsAgentHealth logsagenthealth.Component) {
			if err := healthPlatform.RegisterSubComponent(logsAgentHealth); err != nil {
				log.Errorf("Failed to register logs agent health sub-component: %v", err)
			}
		}),
	)
}
