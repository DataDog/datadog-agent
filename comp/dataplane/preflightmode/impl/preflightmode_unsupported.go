// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !linux && !windows && !darwin

// Package preflightmodeimpl implements the Agent Data Plane preflight mode component
package preflightmodeimpl

import (
	configcomp "github.com/DataDog/datadog-agent/comp/core/config"
	logcomp "github.com/DataDog/datadog-agent/comp/core/log/def"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	preflightmode "github.com/DataDog/datadog-agent/comp/dataplane/preflightmode/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
)

// Requires defines the dependencies for the ADP preflight mode component
type Requires struct {
	Lc        compdef.Lifecycle
	Config    configcomp.Component
	Log       logcomp.Component
	Telemetry telemetry.Component
}

// Provides defines what this component provides
type Provides struct {
	Comp preflightmode.Component
}

type preflightModeComponent struct{}

// NewComponent creates an inert ADP preflight mode component. The Agent Data Plane is only
// shipped for Linux, macOS and Windows, so there is nothing to pre-flight elsewhere.
func NewComponent(_ Requires) Provides {
	return Provides{Comp: &preflightModeComponent{}}
}
