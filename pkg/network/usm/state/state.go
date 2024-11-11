// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package state provides the state of the USM monitor.
package state

import (
	"go.uber.org/atomic"
)

// MonitorState represents the state of the USM monitor.
type MonitorState = string

const (
	// Disabled represents the state of the USM monitor when it is disabled.
	Disabled MonitorState = "Disabled"
	// Running represents the state of the USM monitor when it is running.
	Running MonitorState = "Running"
	// NotRunning represents the state of the USM monitor when it is not running.
	NotRunning MonitorState = "Not running"
	// Stopped represents the state of the USM monitor when it is stopped.
	Stopped MonitorState = "Stopped"
)

var (
	globalState = &atomic.Value{}
)

func init() {
	Set(Disabled)
}

// Set sets the current state of the USM monitor.
func Set(state MonitorState) {
	globalState.Store(state)
}

// Get returns the current state of the USM monitor.
func Get() MonitorState {
	return globalState.Load().(MonitorState)
}
