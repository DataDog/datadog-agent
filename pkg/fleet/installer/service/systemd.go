// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package service provides a way to interact with os services
package service

import (
	"context"
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

func stopUnit(ctx context.Context, unit string, args ...string) error {
	span, _ := tracer.StartSpanFromContext(ctx, "stop_unit")
	defer span.Finish()
	span.SetTag("unit", unit)
	args = append([]string{"stop", unit}, args...)
	return exec.CommandContext(ctx, "systemctl", args...).Run()
}

func startUnit(ctx context.Context, unit string, args ...string) error {
	span, _ := tracer.StartSpanFromContext(ctx, "start_unit")
	defer span.Finish()
	span.SetTag("unit", unit)
	args = append([]string{"start", unit}, args...)
	return exec.CommandContext(ctx, "systemctl", args...).Run()
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
	content, err := embedded.FS.ReadFile(unit)
	if err != nil {
		return fmt.Errorf("error reading embedded unit %s: %w", unit, err)
	}
	unitPath := filepath.Join(systemdPath, unit)
	return os.WriteFile(unitPath, content, 0644)
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
