// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package sysvinit provides a set of functions to manage sysvinit services
package sysvinit

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"go.uber.org/multierr"
)

// Install installs a sys-v init script using update-rc.d
func Install(ctx context.Context, name string) error {
	return telemetry.CommandContext(ctx, "update-rc.d", name, "defaults").Run()
}

// InstallAll installs all sys-v init scripts using update-rc.d
func InstallAll(ctx context.Context, names ...string) error {
	var errs error
	for _, name := range names {
		err := Install(ctx, name)
		errs = multierr.Append(errs, err)
	}
	return errs
}

// Remove removes a sys-v init script using update-rc.d
func Remove(ctx context.Context, name string) error {
	return telemetry.CommandContext(ctx, "update-rc.d", "-f", name, "remove").Run()
}

// RemoveAll removes all sys-v init scripts using update-rc.d
func RemoveAll(ctx context.Context, names ...string) error {
	var errs error
	for _, name := range names {
		err := Remove(ctx, name)
		errs = multierr.Append(errs, err)
	}
	return errs
}

// Restart restarts a sys-v init script using service
func Restart(ctx context.Context, name string) error {
	return telemetry.CommandContext(ctx, "service", name, "restart").Run()
}

// Stop stops a sys-v init script using service
func Stop(ctx context.Context, name string) error {
	return telemetry.CommandContext(ctx, "service", name, "stop").Run()
}

// StopAll stops all sys-v init scripts using service
func StopAll(ctx context.Context, names ...string) error {
	var errs error
	for _, name := range names {
		err := Stop(ctx, name)
		errs = multierr.Append(errs, err)
	}
	return errs
}
