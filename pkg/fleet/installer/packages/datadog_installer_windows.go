// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package packages contains the install/upgrades/uninstall logic for packages
package packages

import (
	"context"
	"fmt"
)

// PrepareInstaller prepares the installer
func PrepareInstaller(_ context.Context) error {
	return fmt.Errorf("Installer Package is not supported on Windows")
}

// SetupInstaller installs and starts the installer
func SetupInstaller(_ context.Context) error {
	return fmt.Errorf("Installer Package is not supported on Windows")
}

// RemoveInstaller removes the installer
func RemoveInstaller(_ context.Context) error {
	return fmt.Errorf("Installer Package is not supported on Windows")
}

// StartInstallerExperiment starts the installer experiment
func StartInstallerExperiment(_ context.Context) error {
	return fmt.Errorf("Installer Package is not supported on Windows")
}

// StopInstallerExperiment stops the installer experiment
func StopInstallerExperiment(_ context.Context) error {
	return fmt.Errorf("Installer Package is not supported on Windows")
}

// PromoteInstallerExperiment promotes the installer experiment
func PromoteInstallerExperiment(_ context.Context) error {
	return fmt.Errorf("Installer Package is not supported on Windows")
}
