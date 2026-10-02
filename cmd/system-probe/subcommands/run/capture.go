// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package run

import (
	"context"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// The daemon owns one manager shared by its API and all producing modules.
// Modules advertise capabilities only when their corresponding sender is running.
func newCaptureManager(lifecycle fx.Lifecycle) *telemetrycapture.Manager {
	manager := telemetrycapture.NewManager("system-probe", version.AgentVersion, version.FullCommit)
	lifecycle.Append(fx.Hook{OnStop: func(context.Context) error {
		manager.Close()
		return nil
	}})
	return manager
}
