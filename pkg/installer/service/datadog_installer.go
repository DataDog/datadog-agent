// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package service

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	installerUnit    = "datadog-installer.service"
	installerUnitExp = "datadog-installer-exp.service"
)

var installerUnits = []string{installerUnit, installerUnitExp}

// SetupInstallerUnits installs and starts the installer systemd units
func SetupInstallerUnits(ctx context.Context) (err error) {
	defer func() {
		if err != nil {
			log.Errorf("Failed to setup installer units: %s, reverting", err)
		}
	}()

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
	return nil
}

// StartInstallerStable starts the stable systemd units for the installer
func StartInstallerStable(ctx context.Context) (err error) {
	return startUnit(ctx, installerUnit)
}

// RemoveInstallerUnits removes the installer systemd units
func RemoveInstallerUnits(ctx context.Context) {
	var err error
	for _, unit := range installerUnits {
		if err = disableUnit(ctx, unit); err != nil {
			log.Warnf("Failed to disable %s: %s", unit, err)
		}
		if err = removeUnit(ctx, unit); err != nil {
			log.Warnf("Failed to stop %s: %s", unit, err)
		}
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
