// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package deployment resolves the effective PAR deployment mode.
package deployment

import (
	"os"
	"path/filepath"
	"runtime"

	configenv "github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
)

// Mode identifies the selected PAR deployment architecture.
type Mode string

const (
	// Monolithic runs PAR in the Go process when split deployment is unavailable.
	Monolithic Mode = "monolithic"
	// Split runs the Rust control plane with an on-demand Go executor.
	Split Mode = "split"
)

// Resolve applies the same deployment policy to standalone startup and the core startup gate.
func Resolve(cfg model.Reader) Mode {
	return resolve(cfg, runtime.GOOS, configenv.IsContainerized(), supervisorAvailable)
}

func resolve(cfg model.Reader, goos string, containerized bool, available func(string, bool) bool) Mode {
	if !(goos == "linux" || (goos == "windows" && !containerized && cfg.GetBool("process_manager.enabled"))) ||
		!available(goos, containerized) {
		return Monolithic
	}
	return Split
}

func supervisorAvailable(goos string, containerized bool) bool {
	// Legacy launchers set this only after checking that they cannot start the supervisor.
	if os.Getenv("DD_INTERNAL_PAR_NO_PROCMGR") == "true" {
		return false
	}
	if containerized {
		// Both supported container launchers pass the supervisor socket to children.
		// Older charts run the standalone binary directly, without the shared IPC setup.
		return os.Getenv("DD_PM_SOCKET_PATH") != ""
	}
	// Check what is installed and configured, not whether the daemon is already
	// listening. A slow or restarting supervisor must not create a second runner.
	return hasHostSupervisor(defaultpaths.GetInstallPath(), goos)
}

func hasHostSupervisor(installDir, goos string) bool {
	var binaries []string
	switch goos {
	case "windows":
		binaries = []string{"bin/agent/dd-procmgrd.exe", "bin/agent/par-control.exe"}
	case "linux":
		binaries = []string{"embedded/bin/dd-procmgrd", "embedded/bin/par-control"}
	default:
		return false
	}
	for _, name := range binaries {
		if !regularFile(filepath.Join(installDir, name)) {
			return false
		}
	}
	for _, name := range []string{"datadog-agent-par-control", "datadog-agent-action-executor"} {
		path := filepath.Join(installDir, "processes.d", name)
		if !regularFile(path+".yaml") && !regularFile(path+".yml") {
			return false
		}
	}
	return true
}

func regularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
