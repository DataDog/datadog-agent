// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// team: agent-runtimes

// Package flightrecorder provides the Component interface for the flight recorder.
package flightrecorder

// Component is the flight recorder that forwards signal data to the Rust sidecar.
type Component interface {
	// flightrecorder is a leaf component, nothing depends on it
}
