// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build serverless

package agentimpl

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
)

// intakeRoutingState is empty for serverless builds: intake routing is not used.
type intakeRoutingState struct{}

// startIntakeRoutingLocked is a no-op for serverless builds: it returns base unchanged.
func (a *logAgent) startIntakeRoutingLocked(_ context.Context, base *config.Endpoints) *config.Endpoints {
	return base
}

// watchIntakeRoutingLocked is a no-op for serverless builds
func (a *logAgent) watchIntakeRoutingLocked() {
	// No-op: serverless agents don't use intake routing
}

// closeIntakeRoutingLocked resets the intake routing state for serverless builds
func (a *logAgent) closeIntakeRoutingLocked() {
	a.routing = intakeRoutingState{}
}
