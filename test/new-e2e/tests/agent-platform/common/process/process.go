// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package process provides utilities for testing processes
package process

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows"
	componentos "github.com/DataDog/test-infra-definitions/components/os"
)

// IsProcessRunning returns true if process is running
func IsProcessRunning(host *components.RemoteHost, process string) (bool, error) {
	os := host.OSFamily
	if os == componentos.LinuxFamily {
		return isProcessRunningUnix(host, process)
	} else if os == componentos.WindowsFamily {
		return windows.IsProcessRunning(host, process)
	}
	return false, fmt.Errorf("unsupported OS type: %v", os)
}

// FindPID returns list of PIDs that match process
func FindPID(host *components.RemoteHost, process string) ([]int, error) {
	os := host.OSFamily
	if os == componentos.LinuxFamily {
		return findPIDUnix(host, process)
	} else if os == componentos.WindowsFamily {
		return windows.FindPID(host, process)
	}
	return nil, fmt.Errorf("unsupported OS type: %v", os)
}
