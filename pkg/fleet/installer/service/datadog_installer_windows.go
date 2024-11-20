// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package service

import (
	"context"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/fleet/internal/msi"
)

const (
	datadogInstaller = "datadog-installer"
)

// SetupInstaller installs and starts the installer
func SetupInstaller(_ context.Context) (err error) {
	cmd, err := msi.Cmd(msi.Install(), msi.WithMsiFromPackagePath("stable", datadogInstaller))
	if err != nil {
		return fmt.Errorf("failed to setup installer: %w", err)
	}
	output, err := cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to setup installer: %w\n%s", err, string(output))
	}
	return nil
}

// RemoveInstaller removes the installer
func RemoveInstaller(_ context.Context) (err error) {
	return msi.RemoveProduct("Datadog Installer")
}

// StartInstallerExperiment starts the installer experiment
func StartInstallerExperiment(_ context.Context) (err error) {
	cmd, err := msi.Cmd(msi.Install(), msi.WithMsiFromPackagePath("experiment", datadogInstaller))
	if err != nil {
		return fmt.Errorf("failed to start installer experiment: %w", err)
	}
	// Launch the msiexec process asynchronously.
	return cmd.FireAndForget()
}

// StopInstallerExperiment stops the installer experiment
func StopInstallerExperiment(_ context.Context) (err error) {
	cmd, err := msi.Cmd(msi.Install(), msi.WithMsiFromPackagePath("stable", datadogInstaller))
	if err != nil {
		return fmt.Errorf("failed to stop installer experiment: %w", err)
	}
	// Launch the msiexec process asynchronously.
	return cmd.FireAndForget()
}

// PromoteInstallerExperiment promotes the installer experiment
func PromoteInstallerExperiment(_ context.Context) error {
	return nil
}
