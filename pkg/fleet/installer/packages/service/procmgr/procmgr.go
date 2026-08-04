// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package procmgr provides a set of functions to manage dd-procmgrd, the Datadog process manager.
//
// dd-procmgrd is itself hosted by a systemd unit, so unit-level operations delegate to the systemd
// package. The payloads it supervises are declared as one YAML file per process in the install
// root's processes.d directory, which is what DD_PM_CONFIG_DIR points the daemon at.
package procmgr

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/service/systemd"
)

const (
	// ProcessesDirName is the per-install directory holding one YAML per supervised process.
	// It's the equivalent of systemd's .service files, but for processes managed by procmgr.
	ProcessesDirName = "processes.d"

	daemonRelPath = "embedded/bin/dd-procmgrd"
)

// IsInstalled reports whether binary dd-procmgrd exists
func IsInstalled(installRoot string) bool {
	fi, err := os.Stat(filepath.Join(installRoot, daemonRelPath))
	if err == nil && !fi.IsDir() {
		return true
	}
	return false
}

// ConfigDir returns the processes.d directory for an install root.
func ConfigDir(installRoot string) string {
	return filepath.Join(installRoot, ProcessesDirName)
}

// WriteConfig writes a single processes.d entry, creating the directory if needed.
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

// Reload reload the systemd unit
func Reload(ctx context.Context) error {
	return systemd.Reload(ctx)
}

// EnableUnit enables the systemd unit
func EnableUnit(ctx context.Context, unit string) error {
	return systemd.EnableUnit(ctx, unit)
}

// DisableUnits disables multiple units.
func DisableUnits(ctx context.Context, units ...string) error {
	return systemd.DisableUnits(ctx, units...)
}

// StartUnit starts a unit.
func StartUnit(ctx context.Context, unit string) error {
	return systemd.StartUnit(ctx, unit)
}

// StopUnits stops multiple units. Stopping the unit hosting dd-procmgrd also stops the processes it
// supervises: the daemon shuts them down in reverse startup order.
func StopUnits(ctx context.Context, units ...string) error {
	return systemd.StopUnits(ctx, units...)
}

// RestartUnit restarts a unit. Restarting the Agent's main unit cycles dd-procmgrd through its
// BindsTo/Wants relationship, and the fresh daemon re-reads every definition and re-evaluates
// condition_path_exists.
func RestartUnit(ctx context.Context, unit string) error {
	return systemd.RestartUnit(ctx, unit)
}
