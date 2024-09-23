// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package service provides a way to interact with os services
package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/service/embedded"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

const systemdPath = "/etc/systemd/system"

func stopUnit(ctx context.Context, unit string, args ...string) (err error) {
	span, _ := tracer.StartSpanFromContext(ctx, "stop_unit")
	defer func() { span.Finish(tracer.WithError(err)) }()
	span.SetTag("unit", unit)
	args = append([]string{"stop", unit}, args...)
	err = exec.CommandContext(ctx, "systemctl", args...).Run()
	exitErr := &exec.ExitError{}
	if !errors.As(err, &exitErr) {
		return err
	}
	span.SetTag("exit_code", exitErr.ExitCode())
	if exitErr.ExitCode() == 5 {
		// exit code 5 means the unit is not loaded, we can continue
		return nil
	}
	return errors.New(string(exitErr.Stderr))
}

func startUnit(ctx context.Context, unit string, args ...string) (err error) {
	span, _ := tracer.StartSpanFromContext(ctx, "start_unit")
	defer func() { span.Finish(tracer.WithError(err)) }()
	span.SetTag("unit", unit)
	args = append([]string{"start", unit}, args...)
	err = exec.CommandContext(ctx, "systemctl", args...).Run()
	exitErr := &exec.ExitError{}
	if !errors.As(err, &exitErr) {
		return err
	}
	span.SetTag("exit_code", exitErr.ExitCode())
	return errors.New(string(exitErr.Stderr))
}

func enableUnit(ctx context.Context, unit string) (err error) {
	span, _ := tracer.StartSpanFromContext(ctx, "enable_unit")
	defer func() { span.Finish(tracer.WithError(err)) }()
	span.SetTag("unit", unit)
	err = exec.CommandContext(ctx, "systemctl", "enable", unit).Run()
	exitErr := &exec.ExitError{}
	if !errors.As(err, &exitErr) {
		return err
	}
	span.SetTag("exit_code", exitErr.ExitCode())
	return errors.New(string(exitErr.Stderr))
}

func disableUnit(ctx context.Context, unit string) (err error) {
	span, _ := tracer.StartSpanFromContext(ctx, "disable_unit")
	defer func() { span.Finish(tracer.WithError(err)) }()
	span.SetTag("unit", unit)

	enabledErr := exec.CommandContext(ctx, "systemctl", "is-enabled", "--quiet", unit).Run()
	if enabledErr != nil {
		// unit is already disabled or doesn't exist, we can return fast
		return nil
	}

	err = exec.CommandContext(ctx, "systemctl", "disable", unit).Run()
	exitErr := &exec.ExitError{}
	if !errors.As(err, &exitErr) {
		return err
	}
	span.SetTag("exit_code", exitErr.ExitCode())
	if exitErr.ExitCode() == 5 {
		// exit code 5 means the unit is not loaded, we can continue
		return nil
	}
	return errors.New(string(exitErr.Stderr))
}

func loadUnit(ctx context.Context, unit string) (err error) {
	span, _ := tracer.StartSpanFromContext(ctx, "load_unit")
	defer func() { span.Finish(tracer.WithError(err)) }()
	span.SetTag("unit", unit)
	content, err := embedded.FS.ReadFile(unit)
	if err != nil {
		return fmt.Errorf("error reading embedded unit %s: %w", unit, err)
	}
	unitPath := filepath.Join(systemdPath, unit)
	return os.WriteFile(unitPath, content, 0644)
}

func removeUnit(ctx context.Context, unit string) (err error) {
	span, _ := tracer.StartSpanFromContext(ctx, "remove_unit")
	defer func() { span.Finish(tracer.WithError(err)) }()
	span.SetTag("unit", unit)
	return os.Remove(path.Join(systemdPath, unit))
}

func systemdReload(ctx context.Context) (err error) {
	span, _ := tracer.StartSpanFromContext(ctx, "systemd_reload")
	defer func() { span.Finish(tracer.WithError(err)) }()
	err = exec.CommandContext(ctx, "systemctl", "daemon-reload").Run()
	exitErr := &exec.ExitError{}
	if !errors.As(err, &exitErr) {
		return err
	}
	span.SetTag("exit_code", exitErr.ExitCode())
	return errors.New(string(exitErr.Stderr))
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
