// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package upstart provides a set of functions to manage upstart services
package upstart

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"go.uber.org/multierr"
)

// Restart restarts an upstart service using initctl
func Restart(ctx context.Context, name string) error {
	errStart := telemetry.CommandContext(ctx, "initctl", "start", name).Run()
	if errStart == nil {
		return nil
	}
	errRestart := telemetry.CommandContext(ctx, "initctl", "restart", name).Run()
	if errRestart == nil {
		return nil
	}
	return fmt.Errorf("failed to restart %s: %w || %w", name, errStart, errRestart)
}

// Stop stops an upstart service using initctl
func Stop(ctx context.Context, name string) error {
	return telemetry.CommandContext(ctx, "initctl", "stop", name).Run()
}

// StopAll stops all upstart services using initctl
func StopAll(ctx context.Context, names ...string) error {
	var errs error
	for _, name := range names {
		err := Stop(ctx, name)
		errs = multierr.Append(errs, err)
	}
	return errs
}
