// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package windows

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
)

// IsProcessRunning returns true if process is running
func IsProcessRunning(host *components.RemoteHost, imageName string) (bool, error) {
	cmd := fmt.Sprintf(`tasklist /fi "ImageName -eq '%s'"`, imageName)
	out, err := host.Execute(cmd)
	if err != nil {
		return false, err
	}
	return strings.Contains(out, imageName), nil
}

// FindPID returns a list of PIDs for processes that match the given pattern
func FindPID(host *components.RemoteHost, pattern string) ([]int, error) {
	cmd := fmt.Sprintf("Get-Process -Name '%s' | Select-Object -ExpandProperty Id", pattern)
	out, err := host.Execute(cmd)
	if err != nil {
		return nil, err
	}
	var pids []int
	for _, strPID := range strings.Split(out, "\n") {
		strPID = strings.TrimSpace(strPID)
		if strPID == "" {
			continue
		}
		pid, err := strconv.Atoi(strPID)
		if err != nil {
			return nil, err
		}
		pids = append(pids, pid)
	}
	return pids, nil
}
