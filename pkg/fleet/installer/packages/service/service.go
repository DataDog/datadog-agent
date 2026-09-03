// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && !darwin

// Package service provides service manager utilities
package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
)

// Type is the service manager type
type Type string

const (
	// UnknownType is returned when the service manager type is not identified
	UnknownType Type = "unknown"
	// SysvinitType is returned when the service manager is sysvinit
	SysvinitType Type = "sysvinit"
	// UpstartType is returned when the service manager is upstart
	UpstartType Type = "upstart"
	// SystemdType is returned when the service manager is systemd
	SystemdType Type = "systemd"
	// ProcmgrType is returned when systemd is present and procmgr is installed + enabled
	// It will have its own logic, either delegated to systemd or managed by procmgr.
	ProcmgrType Type = "procmgr"
)

const (
	// ProcessesDirName is the per-install directory holding one YAML per supervised process.
	// It's the equivalent of systemd's .service files, but for processes managed by procmgr.
	ProcessesDirName = "processes.d"

	procmgrDaemonRelPath = "embedded/bin/dd-procmgrd"
)

// initSystemType memoizes the init system probe. It never returns ProcmgrType: procmgr is a layer
// on top of systemd, not an init system.
var initSystemType = sync.OnceValue(detectInitSystem)

// procmgrEnabled and procmgrInstalled are indirected so tests can drive the selection.
var (
	procmgrEnabled   = func() bool { return env.FromEnv().ProcessManagerEnabled }
	procmgrInstalled = isProcmgrInstalled
)

// GetServiceManagerType returns the service manager of the current system.
//
// procmgr is selected over plain systemd when the init system is systemd, the operator has not
// opted out via DD_PROCESS_MANAGER_ENABLED=false, and procmgr is actually installed. Only the init
// system probe is memoized: the other two change during an install, so they are re-evaluated on
// every call.
func GetServiceManagerType(packagePath string) Type {
	base := initSystemType()
	if base != SystemdType {
		return base
	}
	if !procmgrEnabled() || !procmgrInstalled(packagePath) {
		return SystemdType
	}
	return ProcmgrType
}

// isProcmgrInstalled reports whether binary dd-procmgrd exists.
func isProcmgrInstalled(installRoot string) bool {
	fi, err := os.Stat(filepath.Join(installRoot, procmgrDaemonRelPath))
	if err == nil && !fi.IsDir() {
		return true
	}
	return false
}

// ConfigDir returns the processes.d directory for an install root.
func ConfigDir(installRoot string) string {
	return filepath.Join(installRoot, ProcessesDirName)
}

// WriteProcess writes a single processes.d entry, creating the directory if needed.
func WriteProcess(installRoot string, name string, content []byte) error {
	dir := ConfigDir(installRoot)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create %s: %w", dir, err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, content, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	return nil
}

func detectInitSystem() Type {
	_, err := exec.LookPath("systemctl")
	if err == nil {
		return SystemdType
	}
	_, err = exec.LookPath("initctl")
	if err == nil {
		return UpstartType
	}
	_, err = exec.LookPath("update-rc.d")
	if err == nil {
		return SysvinitType
	}
	return UnknownType
}
