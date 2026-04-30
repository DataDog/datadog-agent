// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package procmgr contains small helpers for interacting with dd-procmgrd from
// the fleet installer (Linux only).
package procmgr

import (
	"context"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/service/systemd"
)

// WriteConfig writes a single YAML process definition into processes.d.
func WriteConfig(dir, name, content string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	path := filepath.Join(dir, name)
	return os.WriteFile(path, []byte(content), 0644)
}

// RemoveConfig removes one YAML file from processes.d if present.
func RemoveConfig(dir, name string) {
	_ = os.Remove(filepath.Join(dir, name))
}

// RestartDaemon asks systemd to restart dd-procmgrd (stable or experiment).
func RestartDaemon(ctx context.Context, experiment bool) error {
	unit := "datadog-agent-procmgrd.service"
	if experiment {
		unit = "datadog-agent-procmgrd-exp.service"
	}
	return systemd.RestartUnit(ctx, unit)
}

// The following helpers delegate to systemctl for ProcmgrType installer hooks
// (unit files still exist; dd-procmgrd supervises children per processes.d).

// EnableUnit runs systemctl enable for a unit file.
func EnableUnit(ctx context.Context, unit string) error {
	return systemd.EnableUnit(ctx, unit)
}

// DisableUnits runs systemctl disable for each unit.
func DisableUnits(ctx context.Context, units ...string) error {
	return systemd.DisableUnits(ctx, units...)
}

// RestartUnit runs systemctl restart for a unit.
func RestartUnit(ctx context.Context, unit string) error {
	return systemd.RestartUnit(ctx, unit)
}

// StopUnits runs systemctl stop for each unit.
func StopUnits(ctx context.Context, units ...string) error {
	return systemd.StopUnits(ctx, units...)
}

// StartUnit runs systemctl start for a unit.
func StartUnit(ctx context.Context, unit string) error {
	return systemd.StartUnit(ctx, unit)
}
