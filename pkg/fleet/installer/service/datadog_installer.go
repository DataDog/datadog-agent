// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package service

import (
	"context"
	"errors"
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

// PreSetupInstaller creates the necessary directories for the installer to be installed.
// FIXME: This is a preinst and I feel bad about it
func PreSetupInstaller() error {
	err := os.MkdirAll("/opt/datadog-packages", 0755)
	if err != nil {
		return fmt.Errorf("error creating /opt/datadog-packages: %w", err)
	}
	return nil
}

func addDDAgentUser(ctx context.Context) error {
	if _, err := user.Lookup("dd-agent"); err == nil {
		return nil
	}
	return exec.CommandContext(ctx, "useradd", "--system", "--shell", "/usr/sbin/nologin", "--home", "/opt/datadog-packages", "--no-create-home", "--no-user-group", "-g", "dd-agent", "dd-agent").Run()
}

func addDDAgentGroup(ctx context.Context) error {
	if _, err := user.LookupGroup("dd-agent"); err == nil {
		return nil
	}
	return exec.CommandContext(ctx, "groupadd", "--system", "dd-agent").Run()
}

// SetupInstaller installs and starts the installer systemd units
func SetupInstaller(ctx context.Context) (err error) {
	defer func() {
		if err != nil {
			log.Errorf("Failed to setup installer: %s, reverting", err)
			RemoveInstaller(ctx)
		}
	}()

	if err = addDDAgentGroup(ctx); err != nil {
		return fmt.Errorf("error creating dd-agent group: %w", err)
	}
	if addDDAgentUser(ctx) != nil {
		return fmt.Errorf("error creating dd-agent user: %w", err)
	}
	err = exec.CommandContext(ctx, "usermod", "-g", "dd-agent", "dd-agent").Run()
	if err != nil {
		return fmt.Errorf("error adding dd-agent user to dd-agent group: %w", err)
	}
	ddAgentUID, ddAgentGID, err := getAgentIDs()
	if err != nil {
		return fmt.Errorf("error getting dd-agent user and group IDs: %w", err)
	}
	err = os.MkdirAll("/var/log/datadog", 0755)
	if err != nil {
		return fmt.Errorf("error creating /var/log/datadog: %w", err)
	}
	err = os.MkdirAll("/var/run/datadog-packages", 0777)
	if err != nil {
		return fmt.Errorf("error creating /var/run/datadog-packages: %w", err)
	}
	// Locks directory can already be created by a package install
	err = os.Chmod("/var/run/datadog-packages", 0777)
	if err != nil {
		return fmt.Errorf("error changing permissions of /var/run/datadog-packages: %w", err)
	}
	err = os.Chown("/var/log/datadog", ddAgentUID, ddAgentGID)
	if err != nil {
		return fmt.Errorf("error changing owner of /var/log/datadog: %w", err)
	}
	err = os.Chown("/opt/datadog-packages/datadog-installer/stable/run", ddAgentUID, ddAgentGID)
	if err != nil {
		return fmt.Errorf("error changing owner of /opt/datadog-packages/datadog-installer/stable/run: %w", err)
	}

	// Create installer path symlink
	err = os.Symlink("/opt/datadog-packages/datadog-installer/stable/bin/installer/installer", "/usr/bin/datadog-installer")
	if err != nil && errors.Is(err, os.ErrExist) {
		log.Info("Installer symlink already exists, skipping")
	} else if err != nil {
		return fmt.Errorf("error creating symlink to /usr/bin/datadog-installer: %w", err)
	}

	// FIXME(Arthur): enable the daemon unit by default and use the same strategy as the system probe
	if os.Getenv("DD_REMOTE_UPDATES") != "true" {
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

func getAgentIDs() (uid, gid int, err error) {
	ddAgentUser, err := user.Lookup("dd-agent")
	if err != nil {
		return -1, -1, fmt.Errorf("dd-agent user not found: %w", err)
	}
	ddAgentGroup, err := user.LookupGroup("dd-agent")
	if err != nil {
		return -1, -1, fmt.Errorf("dd-agent group not found: %w", err)
	}
	ddAgentUID, err := strconv.Atoi(ddAgentUser.Uid)
	if err != nil {
		return -1, -1, fmt.Errorf("error converting dd-agent UID to int: %w", err)
	}
	ddAgentGID, err := strconv.Atoi(ddAgentGroup.Gid)
	if err != nil {
		return -1, -1, fmt.Errorf("error converting dd-agent GID to int: %w", err)
	}
	return ddAgentUID, ddAgentGID, nil
}

// startInstallerStable starts the stable systemd units for the installer
func startInstallerStable(ctx context.Context) (err error) {
	return startUnit(ctx, installerUnit)
}

// RemoveInstaller removes the installer systemd units
func RemoveInstaller(ctx context.Context) {
	for _, unit := range installerUnits {
		if err := stopUnit(ctx, unit); err != nil {
			log.Warnf("Failed stop unit %s: %s", unit, err)
		}
		if err := disableUnit(ctx, unit); err != nil {
			log.Warnf("Failed to disable %s: %s", unit, err)
		}
		if err := removeUnit(ctx, unit); err != nil {
			log.Warnf("Failed to stop %s: %s", unit, err)
		}
	}

	// Remove symlink
	if err := os.Remove("/usr/bin/datadog-installer"); err != nil {
		log.Warnf("Failed to remove /usr/bin/datadog-installer: %s", err)
	}
}

// StartInstallerExperiment installs the experimental systemd units for the installer
func StartInstallerExperiment(ctx context.Context) error {
	return startUnit(ctx, installerUnitExp)
}

// StopInstallerExperiment starts the stable systemd units for the installer
func StopInstallerExperiment(ctx context.Context) error {
	return startUnit(ctx, installerUnit)
}
