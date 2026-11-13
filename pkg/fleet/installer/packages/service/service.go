// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

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
	// ProcmgrType is returned when procmgr is installed + enabled on a known init system.
	// Its lifecycle is delegated to the underlying init system reported by InitSystemType.
	ProcmgrType Type = "procmgr"
)

const (
	// ProcessesDirName is the per-install directory holding one YAML per supervised process.
	// It's the equivalent of systemd's .service files, but for processes managed by procmgr.
	ProcessesDirName = "processes.d"

	procmgrDaemonRelPath = "embedded/bin/dd-procmgrd"
)

// initSystemType memoizes the init system probe.
var initSystemType = sync.OnceValue(detectInitSystem)

// procmgrEnabled and procmgrInstalled are indirected so tests can drive the selection.
var (
	procmgrEnabled   = func() bool { return env.FromEnv().ProcessManagerEnabled }
	procmgrInstalled = isProcmgrInstalled
)

// GetServiceManagerType returns the service manager of the current system.
//
// Only the init system probe is memoized. The opt-out and the installed
// check change during an install, so they are re-evaluated on every call.
func GetServiceManagerType(packagePath string) Type {
	base := initSystemType()
	if base == UnknownType || !procmgrEnabled() || !procmgrInstalled(packagePath) {
		return base
	}
	return ProcmgrType
}

// InitSystemType returns the init system of the current system.
// It never returns ProcmgrType: procmgr is a layer on top of the init system, not an init system.
func InitSystemType() Type {
	return initSystemType()
}

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
// The write lands with rename, so a reader never sees a truncated definition.
func WriteProcess(installRoot string, name string, content []byte) error {
	dir := ConfigDir(installRoot)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create %s: %w", dir, err)
	}
	path := filepath.Join(dir, name)
	f, err := os.CreateTemp(dir, ".tmp-"+name+"-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary file for %s: %w", path, err)
	}
	tempPath := f.Name()
	removeTemp := true
	defer func() {
		_ = f.Close()
		if removeTemp {
			_ = os.Remove(tempPath)
		}
	}()
	if _, err := f.Write(content); err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	if err := f.Chmod(0644); err != nil {
		return fmt.Errorf("failed to chmod %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("failed to close %s: %w", path, err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("failed to replace %s: %w", path, err)
	}
	removeTemp = false
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
