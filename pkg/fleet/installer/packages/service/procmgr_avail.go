// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package service

import (
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
)

const (
	// ProcmgrDaemonRelPath is dd-procmgrd relative to an agent package install root.
	ProcmgrDaemonRelPath = "embedded/bin/dd-procmgrd"

	classicAgentInstallRoot = "/opt/datadog-agent"
)

// ProcmgrDaemonAt reports whether dd-procmgrd exists under agentInstallRoot.
func ProcmgrDaemonAt(agentInstallRoot string) bool {
	if agentInstallRoot == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(agentInstallRoot, ProcmgrDaemonRelPath))
	return err == nil
}

func procmgrDaemonPresentOnHost() bool {
	if ProcmgrDaemonAt(classicAgentInstallRoot) {
		return true
	}
	for _, channel := range []string{"stable", "experiment"} {
		root := filepath.Join(paths.PackagesPath, "datadog-agent", channel)
		if ProcmgrDaemonAt(root) {
			return true
		}
	}
	return false
}

func procmgrInstallerRoutingEnabled() bool {
	return procmgrDaemonPresentOnHost()
}
