// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package systemd provides a set of functions to manage systemd units
package systemd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// UnitsPath is the path where systemd unit files are stored
	UnitsPath = "/etc/systemd/system"
)

// StopUnits stops multiple systemd units
func StopUnits(ctx context.Context, units ...string) error {
	for _, unit := range units {
		err := StopUnit(ctx, unit)
		if err != nil {
			return err
		}
	}
	return nil
}

// StopUnit starts a systemd unit
func StopUnit(ctx context.Context, unit string, args ...string) error {
	args = append([]string{"stop", unit}, args...)
	err := telemetry.CommandContext(ctx, "systemctl", args...).Run()
	exitErr := &exec.ExitError{}
	if !errors.As(err, &exitErr) {
		return err
	}
	// exit code 5 means the unit is not loaded, we can continue
	if exitErr.ExitCode() == 5 {
		return nil
	}
	return err
}

// StartUnit starts a systemd unit
func StartUnit(ctx context.Context, unit string, args ...string) error {
	args = append([]string{"start", unit}, args...)
	err := telemetry.CommandContext(ctx, "systemctl", args...).Run()
	exitErr := &exec.ExitError{}
	if !errors.As(err, &exitErr) {
		return err
	}
	waitStatus, hasWaitStatus := exitErr.Sys().(syscall.WaitStatus)
	// Handle the cases where we self stop:
	// - Exit code 143 (128 + 15) means the process was killed by SIGTERM. This is unlikely to happen because of Go's exec.
	// - Exit code -1 being returned by exec means the process was killed by a signal. We check the wait status to see if it was SIGTERM.
	if (exitErr.ExitCode() == -1 && hasWaitStatus && waitStatus.Signal() == syscall.SIGTERM) || exitErr.ExitCode() == 143 {
		return nil
	}
	return err
}

// RestartUnit restarts a systemd unit
func RestartUnit(ctx context.Context, unit string, args ...string) error {
	args = append([]string{"restart", unit}, args...)
	return telemetry.CommandContext(ctx, "systemctl", args...).Run()
}

// EnableUnit enables a systemd unit
func EnableUnit(ctx context.Context, unit string) error {
	return telemetry.CommandContext(ctx, "systemctl", "enable", unit).Run()
}

// DisableUnit disables a systemd unit
func DisableUnit(ctx context.Context, unit string) error {
	enabledErr := telemetry.CommandContext(ctx, "systemctl", "is-enabled", "--quiet", unit).Run()
	if enabledErr != nil {
		// unit is already disabled or doesn't exist, we can return fast
		return nil
	}

	err := telemetry.CommandContext(ctx, "systemctl", "disable", unit).Run()
	exitErr := &exec.ExitError{}
	if !errors.As(err, &exitErr) {
		return err
	}
	if exitErr.ExitCode() == 5 {
		// exit code 5 means the unit is not loaded, we can continue
		return nil
	}
	return err
}

// WriteEmbeddedUnitsAndReload writes a systemd unit from embedded resources and reloads the systemd daemon
func WriteEmbeddedUnitsAndReload(ctx context.Context, units ...string) (err error) {
	for _, unit := range units {
		err = WriteEmbeddedUnit(ctx, unit)
		if err != nil {
			return err
		}
	}
	return Reload(ctx)
}

// WriteEmbeddedUnit writes a systemd unit from embedded resources
func WriteEmbeddedUnit(ctx context.Context, unit string) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "write_embedded_unit")
	defer func() { span.Finish(err) }()
	span.SetTag("unit", unit)
	content, err := embedded.FS.ReadFile(unit)
	if err != nil {
		return fmt.Errorf("error reading embedded unit %s: %w", unit, err)
	}
	err = os.MkdirAll(UnitsPath, 0755)
	if err != nil {
		return fmt.Errorf("error creating systemd directory: %w", err)
	}
	unitPath := filepath.Join(UnitsPath, unit)
	return os.WriteFile(unitPath, content, 0644)
}

// RemoveUnits removes multiple systemd units
func RemoveUnits(ctx context.Context, units ...string) error {
	for _, unit := range units {
		err := RemoveUnit(ctx, unit)
		if err != nil {
			return err
		}
	}
	return nil
}

// RemoveUnit removes a systemd unit
func RemoveUnit(ctx context.Context, unit string) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "remove_unit")
	defer func() { span.Finish(err) }()
	span.SetTag("unit", unit)
	err = os.Remove(path.Join(UnitsPath, unit))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// WriteUnitOverride writes a systemd unit override
func WriteUnitOverride(ctx context.Context, unit string, name string, content string) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "write_unit_override")
	defer func() { span.Finish(err) }()
	span.SetTag("unit", unit)
	span.SetTag("name", name)
	err = os.MkdirAll(filepath.Join(UnitsPath, unit+".d"), 0755)
	if err != nil {
		return fmt.Errorf("error creating systemd directory: %w", err)
	}
	overridePath := filepath.Join(UnitsPath, unit+".d", fmt.Sprintf("%s.conf", name))
	return os.WriteFile(overridePath, []byte(content), 0644)
}

// Reload reloads the systemd daemon
func Reload(ctx context.Context) (err error) {
	return telemetry.CommandContext(ctx, "systemctl", "daemon-reload").Run()
}

// IsRunning checks if systemd is running using the documented way
// https://www.freedesktop.org/software/systemd/man/latest/sd_booted.html#Notes
func IsRunning() (running bool, err error) {
	_, err = os.Stat("/run/systemd/system")
	if os.IsNotExist(err) {
		log.Infof("Installer: systemd is not running, skip unit setup")
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}
