// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package systemd provides a set of functions to manage systemd units
package systemd

import "context"

// Nothing here, solely present for import purposes

// IsRunning checks if systemd is running using the documented way; noop on Windows
func IsRunning() (bool, error) {
	return false, nil
}

// RestartUnit restarts a systemd unit; noop on Windows
func RestartUnit(_ context.Context, _ string) error {
	return nil
}
