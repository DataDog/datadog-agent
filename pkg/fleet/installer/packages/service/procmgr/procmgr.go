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
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"go.uber.org/multierr"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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

func handleSystemdSelfStops(err error) error {
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

// isRunning checks if systemd is running as PID 1 (copied from service/systemd).
func isRunning() (running bool, err error) {
	_, err = os.Stat("/run/systemd/system")
	if os.IsNotExist(err) {
		log.Infof("Installer: systemd is not running, skip unit setup")
		return false, nil
	} else if err != nil {
		return false, err
	}
	comm, readErr := os.ReadFile("/proc/1/comm")
	if readErr != nil {
		log.Infof("Installer: cannot read /proc/1/comm (%v), assuming systemd is not PID 1", readErr)
		return false, nil
	}
	if strings.TrimSpace(string(comm)) != "systemd" {
		log.Infof("Installer: /run/systemd/system exists but PID 1 is %q (not systemd), skip unit setup", strings.TrimSpace(string(comm)))
		return false, nil
	}
	return true, nil
}

func stopUnit(ctx context.Context, unit string, args ...string) error {
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
	return handleSystemdSelfStops(err)
}

func startUnit(ctx context.Context, unit string, args ...string) error {
	running, err := isRunning()
	if err != nil {
		return err
	}
	if !running {
		log.Infof("Installer: systemd not running, skipping start of %s", unit)
		return nil
	}
	args = append([]string{"start", unit}, args...)
	err = telemetry.CommandContext(ctx, "systemctl", args...).Run()
	return handleSystemdSelfStops(err)
}

func restartUnit(ctx context.Context, unit string, args ...string) error {
	running, err := isRunning()
	if err != nil {
		return err
	}
	if !running {
		log.Infof("Installer: systemd not running, skipping restart of %s", unit)
		return nil
	}
	args = append([]string{"restart", unit}, args...)
	err = telemetry.CommandContext(ctx, "systemctl", args...).Run()
	return handleSystemdSelfStops(err)
}

func enableUnit(ctx context.Context, unit string) error {
	running, err := isRunning()
	if err != nil {
		return err
	}
	if !running {
		log.Infof("Installer: systemd not running, skipping enable of %s", unit)
		return nil
	}
	return telemetry.CommandContext(ctx, "systemctl", "enable", unit).Run()
}

func disableUnit(ctx context.Context, unit string) error {
	enabledErr := telemetry.CommandContext(ctx, "systemctl", "is-enabled", "--quiet", unit).Run()
	if enabledErr != nil {
		// unit is already disabled or doesn't exist, we can return fast
		return nil
	}

	err := telemetry.CommandContext(ctx, "systemctl", "disable", "--force", unit).Run()
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

func disableUnits(ctx context.Context, units ...string) error {
	var errs error
	for _, unit := range units {
		err := disableUnit(ctx, unit)
		errs = multierr.Append(errs, err)
	}
	return errs
}

func stopUnits(ctx context.Context, units ...string) error {
	var errs error
	for _, unit := range units {
		err := stopUnit(ctx, unit)
		errs = multierr.Append(errs, err)
	}
	return errs
}

// RestartDaemon asks systemctl to restart dd-procmgrd (stable or experiment).
func RestartDaemon(ctx context.Context, experiment bool) error {
	unit := "datadog-agent-procmgr.service"
	if experiment {
		unit = "datadog-agent-procmgr-exp.service"
	}
	err := restartUnit(ctx, unit)
	exitErr := &exec.ExitError{}
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 5 {
		// Fresh installs can sync DDOT procmgr config before unit files are loaded.
		return nil
	}
	return err
}

// EnableUnit runs systemctl enable for a unit file.
func EnableUnit(ctx context.Context, unit string) error {
	return enableUnit(ctx, unit)
}

// DisableUnits runs systemctl disable for each unit.
func DisableUnits(ctx context.Context, units ...string) error {
	return disableUnits(ctx, units...)
}

// RestartUnit runs systemctl restart for a unit.
func RestartUnit(ctx context.Context, unit string) error {
	return restartUnit(ctx, unit)
}

// StopUnits runs systemctl stop for each unit.
func StopUnits(ctx context.Context, units ...string) error {
	return stopUnits(ctx, units...)
}

// StartUnit runs systemctl start for a unit.
func StartUnit(ctx context.Context, unit string) error {
	return startUnit(ctx, unit)
}
