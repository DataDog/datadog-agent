// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !linux && !windows && !darwin

// Package dummymodeimpl implements the Agent Data Plane dummy mode component
package dummymodeimpl

import (
	configcomp "github.com/DataDog/datadog-agent/comp/core/config"
	logcomp "github.com/DataDog/datadog-agent/comp/core/log/def"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	dummymode "github.com/DataDog/datadog-agent/comp/dataplane/dummymode/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
)

// Requires defines the dependencies for the ADP dummy mode component
type Requires struct {
	Lc        compdef.Lifecycle
	Config    configcomp.Component
	Log       logcomp.Component
	Telemetry telemetry.Component
}

// Provides defines what this component provides
type Provides struct {
	Comp dummymode.Component
}

type dummyModeComponent struct{}

// NewComponent creates an inert ADP dummy mode component. The Agent Data Plane is only
// shipped for Linux, macOS and Windows, so there is nothing to pre-flight elsewhere.
func NewComponent(_ Requires) Provides {
	return Provides{Comp: &dummyModeComponent{}}
}
