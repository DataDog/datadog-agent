// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"log/slog"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/file"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/service/systemd"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/user"
)

var datadogInstallerPackage = hooks{
	preInstall:            preInstallDatadogInstaller,
	postInstall:           postInstallDatadogInstaller,
	preRemove:             preRemoveDatadogInstaller,
	postStartExperiment:   postStartExperimentDatadogInstaller,
	preStopExperiment:     preStopExperimentDatadogInstaller,
	postPromoteExperiment: postPromoteExperimentDatadogInstaller,
}

const (
	installerUnit    = "datadog-installer.service"
	installerUnitExp = "datadog-installer-exp.service"
	installerSymlink = "/usr/bin/datadog-installer"
)

var installerUnits = []string{installerUnit, installerUnitExp}

var (
	installerDirectories = file.Directories{
		{Path: "/opt/datadog-packages/run", Mode: 0755, Owner: "dd-agent", Group: "dd-agent"},
		{Path: "/opt/datadog-packages/tmp", Mode: 0755, Owner: "dd-agent", Group: "dd-agent"},
	}
)

// preInstallDatadogInstaller prepares the installer
func preInstallDatadogInstaller(ctx HookContext) error {
	if err := systemd.StopUnit(ctx, installerUnit); err != nil {
		slog.WarnContext(ctx, "Failed to stop unit %s: %s", installerUnit, err)
	}
	if err := systemd.DisableUnit(ctx, installerUnit); err != nil {
		slog.WarnContext(ctx, "Failed to disable %s: %s", installerUnit, err)
	}
	return nil
}

// postInstallDatadogInstaller installs and starts the installer systemd units
func postInstallDatadogInstaller(ctx HookContext) (err error) {
	defer func() {
		if err != nil {
			slog.ErrorContext(ctx, "Failed to setup installer, reverting", "error", err)
			err = preRemoveDatadogInstaller(ctx)
		}
	}()

	// 1. Ensure the dd-agent user and group exist
	err = user.EnsureAgentUserAndGroup(ctx, "/opt/datadog-packages")
	if err != nil {
		return fmt.Errorf("error ensuring dd-agent user and group: %w", err)
	}
	// 2. Ensure the installer and agent directories exist and have the correct permissions
	if err = installerDirectories.Ensure(ctx); err != nil {
		return fmt.Errorf("error ensuring installer directories: %w", err)
	}
	if err = agentDirectories.Ensure(ctx); err != nil {
		return fmt.Errorf("error ensuring agent directories: %w", err)
	}
	if err = file.EnsureSymlink(ctx, "/opt/datadog-packages/datadog-installer/stable/bin/installer/installer", installerSymlink); err != nil {
		return fmt.Errorf("error creating symlink /usr/bin/datadog-installer: %w", err)
	}
	if err = file.EnsureSymlink(ctx, "/opt/datadog-packages/run", "/var/run/datadog-installer"); err != nil {
		return fmt.Errorf("error creating symlink /var/run/datadog-installer: %w", err)
	}
	// 3. Install the installer systemd units
	systemdRunning, err := systemd.IsRunning(ctx)
	if err != nil {
		return fmt.Errorf("error checking if systemd is running: %w", err)
	}
	if !systemdRunning {
		slog.InfoContext(ctx, "Installer: systemd is not running, skipping unit setup")
		return nil
	}
	if err = writeEmbeddedUnitsAndReload(ctx, installerUnits...); err != nil {
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

// preRemoveDatadogInstaller removes the installer systemd units
func preRemoveDatadogInstaller(ctx HookContext) error {
	for _, unit := range installerUnits {
		if err := systemd.StopUnit(ctx, unit); err != nil {
			exitErr, ok := err.(*exec.ExitError)
			// unit is not installed, avoid noisy warn logs
			// https://www.freedesktop.org/software/systemd/man/latest/systemd.exec.html#Process%20Exit%20Codes
			if ok && exitErr.ExitCode() == 5 {
				continue
			}
			slog.WarnContext(ctx, "Failed stop unit %s: %s", unit, err)
		}
		if err := systemd.DisableUnit(ctx, unit); err != nil {
			slog.WarnContext(ctx, "Failed to disable %s: %s", unit, err)
		}
		if err := removeUnits(ctx, unit); err != nil {
			slog.WarnContext(ctx, "Failed to stop %s: %s", unit, err)
		}
	}

	// Remove symlink
	if err := os.Remove("/usr/bin/datadog-installer"); err != nil {
		slog.WarnContext(ctx, "Failed to remove /usr/bin/datadog-installer", "error", err)
	}

	// TODO: return error to caller?
	return nil
}

// postStartExperimentDatadogInstaller starts the experimental systemd units for the installer
func postStartExperimentDatadogInstaller(ctx HookContext) error {
	return systemd.StartUnit(ctx, installerUnitExp, "--no-block")
}

// preStopExperimentDatadogInstaller stops the stable systemd units for the installer
func preStopExperimentDatadogInstaller(ctx HookContext) error {
	return systemd.StartUnit(ctx, installerUnit)
}

// postPromoteExperimentDatadogInstaller promotes the installer experiment
func postPromoteExperimentDatadogInstaller(ctx HookContext) error {
	return systemd.StartUnit(ctx, installerUnit)
}
