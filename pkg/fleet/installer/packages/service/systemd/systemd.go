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
	"path/filepath"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"go.uber.org/multierr"
)

const (
	userUnitsPath = "/etc/systemd/system"
)

// StopUnits stops multiple systemd units
func StopUnits(ctx context.Context, units ...string) error {
	var errs error
	for _, unit := range units {
		err := StopUnit(ctx, unit)
		errs = multierr.Append(errs, err)
	}
	return errs
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

// DisableUnits disables multiple systemd units
func DisableUnits(ctx context.Context, units ...string) error {
	var errs error
	for _, unit := range units {
		err := DisableUnit(ctx, unit)
		errs = multierr.Append(errs, err)
	}
	return errs
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

// WriteUnitOverride writes a systemd unit override
func WriteUnitOverride(ctx context.Context, unit string, name string, content string) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "write_unit_override")
	defer func() { span.Finish(err) }()
	span.SetTag("unit", unit)
	span.SetTag("name", name)
	err = os.MkdirAll(filepath.Join(userUnitsPath, unit+".d"), 0755)
	if err != nil {
		return fmt.Errorf("error creating systemd directory: %w", err)
	}
	overridePath := filepath.Join(userUnitsPath, unit+".d", fmt.Sprintf("%s.conf", name))
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

// JournaldLogs returns the logs for a given unit since a given time
func JournaldLogs(ctx context.Context, unit string, since time.Time) (string, error) {
	journalctlCmd := exec.CommandContext(ctx, "journalctl", "_COMM=systemd", "--unit", unit, "-e", "--no-pager", "--since", since.Format(time.RFC3339))
	stdout, err := journalctlCmd.Output()
	if err != nil {
		return "", err
	}
	return string(stdout), nil
}
