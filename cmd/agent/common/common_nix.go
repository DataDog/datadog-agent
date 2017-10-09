// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build freebsd netbsd openbsd solaris dragonfly linux

package common

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// DefaultConfPath points to the folder containing datadog.yaml
const DefaultConfPath = "/etc/datadog-agent"

var (
	// PyChecksPath holds the path to the python checks from integrations-core shipped with the agent
	PyChecksPath = filepath.Join(_here, "..", "..", "checks.d")
	// DistPath holds the path to the folder containing distribution files
	distPath = filepath.Join(_here, "dist")
	// ViewPath holds the path to the folder containing the GUI support files
	viewPath = filepath.Join(_here, "..", "..", "cmd", "gui", "view")
)

// GetDistPath returns the fully qualified path to the 'dist' directory
func GetDistPath() string {
	return distPath
}

// GetViewPath returns the fully qualified path to the 'gui/view' directory
func GetViewPath() string {
	return viewPath
}

// Restart is used by the GUI to restart the agent
func Restart() error {
	cmd := exec.Command(filepath.Join(_here, "agent"), "restart")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Start()
	if err != nil {
		return fmt.Errorf("Failed to fork main process. Error: %v", err)
	}

	return nil
}
