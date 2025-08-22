// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

// Package main provides the CGO-exportable version of system-probe
package main

import "C"
import (
	"context"

	systemproberun "github.com/DataDog/datadog-agent/cmd/system-probe/subcommands/run"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

var (
	systemProbeErrCh <-chan error
	stopCtx          context.Context
	stopCancel       context.CancelFunc
)

//export StartSystemProbe
func StartSystemProbe(configPath *C.char, fleetPoliciesDirPath *C.char, pidPath *C.char) C.int {
	// Set flavor
	flavor.SetFlavor(flavor.SystemProbe)

	// Create context for system probe management
	stopCtx, stopCancel = context.WithCancel(context.Background())
	ctxChan := make(chan context.Context, 1)

	// Start system probe with defaults
	var err error
	systemProbeErrCh, err = systemproberun.StartSystemProbeWithDefaults(ctxChan)
	if err != nil {
		return 1 // Failed to start
	}

	// Send context to system probe so it knows how to stop
	ctxChan <- stopCtx

	return 0 // Started successfully
}

//export StopSystemProbe
func StopSystemProbe() C.int {
	if stopCancel != nil {
		stopCancel() // This will cause the system probe to shut down

		// Wait for system probe to actually stop
		if systemProbeErrCh != nil {
			err := <-systemProbeErrCh
			if err != nil {
				return 1 // Error during shutdown
			}
		}
	}
	return 0 // Stopped successfully
}

//export WaitForSystemProbe
func WaitForSystemProbe() C.int {
	if systemProbeErrCh != nil {
		err := <-systemProbeErrCh
		if err != nil {
			return 1 // System probe exited with error
		}
	}
	return 0 // System probe exited cleanly
}

func main() {}
