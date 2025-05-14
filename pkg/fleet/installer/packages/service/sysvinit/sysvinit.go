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
)

// Install installs a sys-v init script using update-rc.d
func Install(ctx context.Context, name string) error {
	return telemetry.CommandContext(ctx, "update-rc.d", name, "defaults").Run()
}

// Restart restarts a sys-v init script using service
func Restart(ctx context.Context, name string) error {
	return telemetry.CommandContext(ctx, "service", name, "restart").Run()
}
