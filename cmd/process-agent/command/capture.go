// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package command

import (
	"context"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// The daemon owns one manager shared by its API and producer components. It is
// deliberately absent from process.Bundle, which is also constructed by core.
func newCaptureManager(lifecycle fx.Lifecycle) *telemetrycapture.Manager {
	manager := telemetrycapture.NewManager("process-agent", version.AgentVersion, version.FullCommit)
	lifecycle.Append(fx.Hook{OnStop: func(context.Context) error {
		manager.Close()
		return nil
	}})
	return manager
}
