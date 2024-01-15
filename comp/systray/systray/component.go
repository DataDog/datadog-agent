// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows

// Package systray provides a component for the system tray application
package systray

// team: windows-agent

// Params for ddtray component
type Params struct {
	LaunchGuiFlag      bool
	LaunchElevatedFlag bool
	LaunchCommand      string
}

// Component for ddtray
type Component interface {
}
