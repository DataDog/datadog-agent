// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package service

import "context"

// SetupInstaller noop
func SetupInstaller(_ context.Context, _ bool) error {
	return nil
}

// RemoveInstaller noop
func RemoveInstaller(_ context.Context) error {
	return nil
}

// StartInstallerExperiment noop
func StartInstallerExperiment(_ context.Context) error {
	return nil
}

// StopInstallerExperiment noop
func StopInstallerExperiment(_ context.Context) error {
	return nil
}

// StartInstallerStable noop
func StartInstallerStable(_ context.Context) error {
	return nil
}
