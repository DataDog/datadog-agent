// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	installerUnit    = "datadog-installer.service"
	installerUnitExp = "datadog-installer-exp.service"
)

var installerUnits = []string{installerUnit, installerUnitExp}

// SetupInstaller installs and starts the installer systemd units
func SetupInstaller(ctx context.Context, enableDaemon bool) (err error) {
	defer func() {
		if err != nil {
			log.Errorf("Failed to setup installer: %s, reverting", err)
			err = RemoveInstaller(ctx)
			if err != nil {
				log.Warnf("Failed to revert installer setup: %s", err)
			}
		}
	}()

	cmd := exec.CommandContext(ctx, "adduser", "--system", "dd-agent", "--disabled-login", "--shell", "/usr/sbin/nologin", "--home", "/opt/datadog-packages", "--no-create-home", "--group", "--quiet")
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("error creating dd-agent user: %w", err)
	}
	cmd = exec.CommandContext(ctx, "addgroup", "--system", "dd-agent", "--quiet")
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("error creating dd-agent group: %w", err)
	}
	cmd = exec.CommandContext(ctx, "usermod", "-g", "dd-agent", "dd-agent")
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("error adding dd-agent user to dd-agent group: %w", err)
	}

	ddAgentUser, err := user.Lookup("dd-agent")
	if err != nil {
		return fmt.Errorf("dd-agent user not found: %w", err)
	}
	ddAgentGroup, err := user.LookupGroup("dd-agent")
	if err != nil {
		return fmt.Errorf("dd-agent group not found: %w", err)
	}
	ddAgentUID, err := strconv.Atoi(ddAgentUser.Uid)
	if err != nil {
		return fmt.Errorf("error converting dd-agent UID to int: %w", err)
	}
	ddAgentGID, err := strconv.Atoi(ddAgentGroup.Gid)
	if err != nil {
		return fmt.Errorf("error converting dd-agent GID to int: %w", err)
	}
	err = os.MkdirAll("/opt/datadog-packages", 0755)
	if err != nil {
		return fmt.Errorf("error creating /opt/datadog-packages: %w", err)
	}
	err = os.MkdirAll("/var/log/datadog", 0755)
	if err != nil {
		return fmt.Errorf("error creating /var/log/datadog: %w", err)
	}
	err = os.MkdirAll("/var/run/datadog-packages", 0755)
	if err != nil {
		return fmt.Errorf("error creating /var/run/datadog-packages: %w", err)
	}
	err = os.Chown("/var/log/datadog", ddAgentUID, ddAgentGID)
	if err != nil {
		return fmt.Errorf("error changing owner of /var/log/datadog: %w", err)
	}
	if !enableDaemon {
		return nil
	}

	// Check if systemd is running, if not return early
	systemdRunning, err := isSystemdRunning()
	if err != nil {
		return fmt.Errorf("error checking if systemd is running: %w", err)
	}
	if !systemdRunning {
		log.Infof("Installer: systemd is not running, skipping unit setup")
		return nil
	}

	for _, unit := range installerUnits {
		if err = loadUnit(ctx, unit); err != nil {
			return err
		}
	}

	if err = systemdReload(ctx); err != nil {
		return err
	}

	if err = enableUnit(ctx, installerUnit); err != nil {
		return err
	}

	return startInstallerStable(ctx)
}

// startInstallerStable starts the stable systemd units for the installer
func startInstallerStable(ctx context.Context) (err error) {
	return startUnit(ctx, installerUnit)
}

// RemoveInstaller removes the installer systemd units
func RemoveInstaller(ctx context.Context) error {
	var err error
	for _, unit := range installerUnits {
		if err = stopUnit(ctx, unit); err != nil {
			return fmt.Errorf("Failed stop unit %s: %s", unit, err)
		}
		if err = disableUnit(ctx, unit); err != nil {
			return fmt.Errorf("Failed to disable %s: %s", unit, err)
		}
		if err = removeUnit(ctx, unit); err != nil {
			return fmt.Errorf("Failed to stop %s: %s", unit, err)
		}
	}
	return nil
}

// StartInstallerExperiment installs the experimental systemd units for the installer
func StartInstallerExperiment(ctx context.Context) error {
	return startUnit(ctx, installerUnitExp)
}

// StopInstallerExperiment starts the stable systemd units for the installer
func StopInstallerExperiment(ctx context.Context) error {
	return startUnit(ctx, installerUnit)
}
