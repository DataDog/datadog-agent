// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package process provides utilities for testing processes
package process

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	windows "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	componentos "github.com/DataDog/test-infra-definitions/components/os"
)

// IsRunning returns true if process is running
func IsRunning(host *components.RemoteHost, processName string) (bool, error) {
	os := host.OSFamily
	if os == componentos.LinuxFamily {
		return isProcessRunningUnix(host, processName)
	} else if os == componentos.WindowsFamily {
		return windows.IsProcessRunning(host, processName)
	}
	return false, fmt.Errorf("unsupported OS type: %v", os)
}

// FindPID returns list of PIDs that match processName
func FindPID(host *components.RemoteHost, processName string) ([]int, error) {
	os := host.OSFamily
	if os == componentos.LinuxFamily {
		return findPIDUnix(host, processName)
	} else if os == componentos.WindowsFamily {
		return windows.FindPID(host, processName)
	}
	return nil, fmt.Errorf("unsupported OS type: %v", os)
}
