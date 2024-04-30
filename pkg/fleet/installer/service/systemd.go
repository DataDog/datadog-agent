// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package service provides a way to interact with os services
package service

import (
	"context"
	"os"
	"os/exec"
	"path"

	"github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

var (
	systemdPath = findSystemdPath()
)

const (
	debSystemdPath = "/lib/systemd/system" // todo load it at build time from omnibus
	rpmSystemdPath = "/usr/lib/systemd/system"
)

// findSystemdPath todo: this is a hacky way to detect on which os family we are currently
// running and finding the correct systemd path.
// We should probably provide the correct path when we build the package
func findSystemdPath() (systemdPath string) {
	if _, err := os.Stat(rpmSystemdPath); err == nil {
		return rpmSystemdPath
	}
	return debSystemdPath
}

// restartUnit restarts a systemd unit
func restartUnit(ctx context.Context, unit string) error {
	// check that the unit exists first
	if _, err := os.Stat(path.Join(systemdPath, unit)); os.IsNotExist(err) {
		log.Infof("Unit %s does not exist, skipping restart", unit)
		return nil
	}

	if err := stopUnit(ctx, unit); err != nil {
		return err
	}
	if err := startUnit(ctx, unit); err != nil {
		return err
	}
	return nil
}

func stopUnit(ctx context.Context, unit string) error {
	span, _ := tracer.StartSpanFromContext(ctx, "stop_unit")
	defer span.Finish()
	span.SetTag("unit", unit)
	return exec.CommandContext(ctx, "systemctl", "stop", unit, "--no-block").Run()
}

func startUnit(ctx context.Context, unit string) error {
	span, _ := tracer.StartSpanFromContext(ctx, "start_unit")
	defer span.Finish()
	span.SetTag("unit", unit)
	return exec.CommandContext(ctx, "systemctl", "start", unit, "--no-block").Run()
}

func enableUnit(ctx context.Context, unit string) error {
	span, _ := tracer.StartSpanFromContext(ctx, "enable_unit")
	defer span.Finish()
	span.SetTag("unit", unit)
	return exec.CommandContext(ctx, "systemctl", "enable", unit).Run()
}

func disableUnit(ctx context.Context, unit string) error {
	span, _ := tracer.StartSpanFromContext(ctx, "disable_unit")
	defer span.Finish()
	span.SetTag("unit", unit)
	return exec.CommandContext(ctx, "systemctl", "disable", unit).Run()
}

func loadUnit(ctx context.Context, unit string) error {
	span, _ := tracer.StartSpanFromContext(ctx, "load_unit")
	defer span.Finish()
	span.SetTag("unit", unit)
	return exec.CommandContext(ctx, "cp", "-f", path.Join(setup.InstallPath, "systemd", unit), path.Join(systemdPath, unit)).Run()
}

func removeUnit(ctx context.Context, unit string) error {
	span, _ := tracer.StartSpanFromContext(ctx, "remove_unit")
	defer span.Finish()
	span.SetTag("unit", unit)
	return os.Remove(path.Join(systemdPath, unit))
}

func systemdReload(ctx context.Context) error {
	span, _ := tracer.StartSpanFromContext(ctx, "systemd_reload")
	defer span.Finish()
	return exec.CommandContext(ctx, "systemctl", "daemon-reload").Run()
}

// isSystemdRunning checks if systemd is running using the documented way
// https://www.freedesktop.org/software/systemd/man/latest/sd_booted.html#Notes
func isSystemdRunning() (running bool, err error) {
	_, err = os.Stat("/run/systemd/system")
	if os.IsNotExist(err) {
		log.Infof("Installer: systemd is not running, skip unit setup")
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}
