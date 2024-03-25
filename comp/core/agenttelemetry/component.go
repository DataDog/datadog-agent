// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// ---------------------------------------------------
//
// This is experimental code and is subject to change.
//
// ---------------------------------------------------

// Package agenttelemetry implements a component to generate Agent telemetry
package agenttelemetry

// team: agent-metrics-logs

// Component is the component type
type Component interface {
	// GetAsJSON returns the payload as a JSON string. Useful to be displayed in the CLI or added to a flare.
	GetAsJSON() ([]byte, error)
}
