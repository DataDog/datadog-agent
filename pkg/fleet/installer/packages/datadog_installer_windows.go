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
	"os"
	"path"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/msi"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
)

const (
	datadogInstaller = "datadog-installer"
)

// PrepareInstaller prepares the installer
func PrepareInstaller(_ context.Context) error {
	return nil
}

// SetupInstaller installs and starts the installer
func SetupInstaller(_ context.Context) error {
	rootPath := ""
	_, err := os.Stat(paths.RootTmpDir)
	// If bootstrap has not been called before, `paths.RootTmpDir` might not exist
	if os.IsExist(err) {
		rootPath = paths.RootTmpDir
	}
	tempDir, err := os.MkdirTemp(rootPath, "datadog-installer")
	if err != nil {
		return err
	}

	msiOptions := []msi.MsiexecOption{
		msi.Install(),
		msi.WithMsiFromPackagePath("stable", datadogInstaller),
		msi.WithLogFile(path.Join(tempDir, "setup_installer.log")),
	}
	ddAgentUsername := os.Getenv("DD_AGENT_USER_NAME")
	if ddAgentUsername != "" {
		msiOptions = append(msiOptions, msi.WithDdAgentUserName(ddAgentUsername))
	}
	cmd, err := msi.Cmd(msiOptions...)
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
func RemoveInstaller(_ context.Context) error {
	return msi.RemoveProduct("Datadog Installer")
}

// StartInstallerExperiment starts the installer experiment
func StartInstallerExperiment(_ context.Context) error {
	tempDir, err := os.MkdirTemp(paths.RootTmpDir, "datadog-installer")
	if err != nil {
		return err
	}

	cmd, err := msi.Cmd(msi.Install(), msi.WithMsiFromPackagePath("experiment", datadogInstaller), msi.WithLogFile(path.Join(tempDir, "start_installer_experiment.log")))
	if err != nil {
		return fmt.Errorf("failed to start installer experiment: %w", err)
	}
	// Launch the msiexec process asynchronously.
	return cmd.FireAndForget()
}

// StopInstallerExperiment stops the installer experiment
func StopInstallerExperiment(_ context.Context) error {
	tempDir, err := os.MkdirTemp(paths.RootTmpDir, "datadog-installer")
	if err != nil {
		return err
	}

	cmd, err := msi.Cmd(msi.Install(), msi.WithMsiFromPackagePath("stable", datadogInstaller), msi.WithLogFile(path.Join(tempDir, "stop_installer_experiment.log")))
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
