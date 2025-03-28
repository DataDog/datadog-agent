// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package packages contains the install/upgrades/uninstall logic for packages
package packages

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/file"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/systemd"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/user"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	installerSymlink = "/usr/bin/datadog-installer"
	installerUnit    = "datadog-installer.service"
	installerUnitExp = "datadog-installer-exp.service"
)

var installerUnits = []string{installerUnit, installerUnitExp}

var (
	installerDirectories = file.Directories{
		{Path: "/opt/datadog-packages/run", Mode: 0755, Owner: "dd-agent", Group: "dd-agent"},
		{Path: "/opt/datadog-packages/tmp", Mode: 0755, Owner: "dd-agent", Group: "dd-agent"},
	}
)

// PrepareInstaller prepares the installer
func PrepareInstaller(ctx context.Context) error {
	if err := systemd.StopUnit(ctx, installerUnit); err != nil {
		log.Warnf("Failed to stop unit %s: %s", installerUnit, err)
	}
	if err := systemd.DisableUnit(ctx, installerUnit); err != nil {
		log.Warnf("Failed to disable %s: %s", installerUnit, err)
	}
	return nil
}

// SetupInstaller installs and starts the installer systemd units
func SetupInstaller(ctx context.Context) (err error) {
	defer func() {
		if err != nil {
			log.Errorf("Failed to setup installer, reverting: %s", err)
			err = RemoveInstaller(ctx)
		}
	}()

	// 1. Ensure the dd-agent user and group exist
	err = user.EnsureAgentUserAndGroup(ctx, "/opt/datadog-packages")
	if err != nil {
		return fmt.Errorf("error ensuring dd-agent user and group: %w", err)
	}
	// 2. Ensure the installer and agent directories exist and have the correct permissions
	if err = installerDirectories.Ensure(); err != nil {
		return fmt.Errorf("error ensuring installer directories: %w", err)
	}
	if err = agentDirectories.Ensure(); err != nil {
		return fmt.Errorf("error ensuring agent directories: %w", err)
	}
	if err = file.EnsureSymlink("/opt/datadog-packages/datadog-installer/stable/bin/installer/installer", installerSymlink); err != nil {
		return fmt.Errorf("error creating symlink /usr/bin/datadog-installer: %w", err)
	}
	if err = file.EnsureSymlink("/opt/datadog-packages/run", "/var/run/datadog-installer"); err != nil {
		return fmt.Errorf("error creating symlink /var/run/datadog-installer: %w", err)
	}
	// 3. Install the installer systemd units
	systemdRunning, err := systemd.IsRunning()
	if err != nil {
		return fmt.Errorf("error checking if systemd is running: %w", err)
	}
	if !systemdRunning {
		log.Infof("Installer: systemd is not running, skipping unit setup")
		return nil
	}
	for _, unit := range installerUnits {
		if err = systemd.WriteEmbeddedUnit(ctx, unit); err != nil {
			return err
		}
	}
	if err = systemd.Reload(ctx); err != nil {
		return err
	}
	if err = systemd.EnableUnit(ctx, installerUnit); err != nil {
		return err
	}
	return startInstallerStable(ctx)
}

// startInstallerStable starts the stable systemd units for the installer
func startInstallerStable(ctx context.Context) (err error) {
	_, err = os.Stat("/etc/datadog-agent/datadog.yaml")
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	// this is expected during a fresh install with the install script / asible / chef / etc...
	// the config is populated afterwards by the install method and the agent is restarted
	if os.IsNotExist(err) {
		return nil
	}
	return systemd.StartUnit(ctx, installerUnit)
}

// RemoveInstaller removes the installer systemd units
func RemoveInstaller(ctx context.Context) error {
	for _, unit := range installerUnits {
		if err := systemd.StopUnit(ctx, unit); err != nil {
			exitErr, ok := err.(*exec.ExitError)
			// unit is not installed, avoid noisy warn logs
			// https://www.freedesktop.org/software/systemd/man/latest/systemd.exec.html#Process%20Exit%20Codes
			if ok && exitErr.ExitCode() == 5 {
				continue
			}
			log.Warnf("Failed stop unit %s: %s", unit, err)
		}
		if err := systemd.DisableUnit(ctx, unit); err != nil {
			log.Warnf("Failed to disable %s: %s", unit, err)
		}
		if err := systemd.RemoveUnit(ctx, unit); err != nil {
			log.Warnf("Failed to stop %s: %s", unit, err)
		}
	}

	// Remove symlink
	if err := os.Remove("/usr/bin/datadog-installer"); err != nil {
		log.Warnf("Failed to remove /usr/bin/datadog-installer: %s", err)
	}

	// TODO: return error to caller?
	return nil
}

// StartInstallerExperiment installs the experimental systemd units for the installer
func StartInstallerExperiment(ctx context.Context) error {
	return systemd.StartUnit(ctx, installerUnitExp, "--no-block")
}

// StopInstallerExperiment starts the stable systemd units for the installer
func StopInstallerExperiment(ctx context.Context) error {
	return systemd.StartUnit(ctx, installerUnit)
}

// PromoteInstallerExperiment promotes the installer experiment
func PromoteInstallerExperiment(ctx context.Context) error {
	return StopInstallerExperiment(ctx)
}
