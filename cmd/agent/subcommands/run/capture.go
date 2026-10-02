// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package run

import (
	"context"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
	"github.com/DataDog/datadog-agent/pkg/version"
	"go.uber.org/fx"
)

// newCaptureManager is owned by this daemon's dependency graph. Every tee and
// its authenticated API receive this same dormant instance.
func newCaptureManager(lc fx.Lifecycle) *telemetrycapture.Manager {
	manager := telemetrycapture.NewManager("core-agent", version.AgentVersion, version.FullCommit)
	lc.Append(fx.Hook{OnStop: func(context.Context) error { manager.Close(); return nil }})
	return manager
}
