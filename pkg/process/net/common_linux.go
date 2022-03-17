// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package net

import (
	"fmt"
	"os"
)

const (
	connectionsURL = "http://unix/connections"
	statsURL       = "http://unix/debug/stats"
	procStatsURL   = "http://unix/proc/stats"
	registerURL    = "http://unix/register"
	netType        = "unix"
)

// CheckPath is used in conjunction with calling the stats endpoint, since we are calling this
// From the main agent and want to ensure the socket exists
func CheckPath() error {
	if globalSocketPath == "" {
		return fmt.Errorf("remote tracer has no path defined")
	}

	if _, err := os.Stat(globalSocketPath); err != nil {
		return fmt.Errorf("socket path does not exist: %v", err)
	}
	return nil
}
