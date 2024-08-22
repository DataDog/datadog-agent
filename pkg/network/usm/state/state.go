// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package state provides the state of the USM monitor.
package state

// MonitorState represents the state of the USM monitor.
type MonitorState = string

const (
	Disabled   MonitorState = "Disabled"
	Running    MonitorState = "Running"
	NotRunning MonitorState = "Not running"
	Stopped    MonitorState = "Stopped"
)

var (
	State = Disabled
)

// Set sets the current state of the USM monitor.
func Set(state MonitorState) {
	State = state
}

// Get returns the current state of the USM monitor.
func Get() MonitorState {
	return State
}
