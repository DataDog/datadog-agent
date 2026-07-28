// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package dummymode pre-flights the Agent Data Plane (ADP).
//
// When the operator has expressed no opinion about ADP, this runs it once at Agent startup for
// a short window in an inert configuration — a throwaway DogStatsD endpoint under the run
// directory, so it handles no customer data and never takes the Agent's own DogStatsD port —
// pushes one throwaway metric through it, then stops it and scans its output for errors. The
// point is to find environment-specific ADP problems before ADP is enabled for real.
//
// See isEligible in comp/dataplane/dummymode/impl for exactly when it runs, and
// docs/dev/agent-data-plane.md for the design.
package dummymode

// team: agent-data-plane

// Component has no methods: the run is driven entirely from fx.Lifecycle hooks.
type Component interface {
}
