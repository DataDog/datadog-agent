// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package npschedulerimpl

// TODO: Remove if not needed

// TracerouteRunnerType defines the type of traceroute runner (classic or simple)
type TracerouteRunnerType int

const (
	// ClassicTraceroute correspond to the classic Traceroute Runner that depend on build tags.
	ClassicTraceroute TracerouteRunnerType = iota

	// SimpleTraceroute will instruct to run directly the plain traceroute Runner
	SimpleTraceroute
)

// Params provides the kind of agent we're instantiating npscheduler for
type Params struct {
	Enabled          bool
	TracerouteRunner TracerouteRunnerType
}
