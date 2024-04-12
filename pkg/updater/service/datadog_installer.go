// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package service

import "github.com/DataDog/datadog-agent/pkg/util/log"

const (
	installerUnit    = "datadog-installer.service"
	installerUnitExp = "datadog-installer-exp.service"
)

var installerUnits = []string{installerUnit, installerUnitExp}

// SetupInstallerUnit installs and starts the installer systemd units
func SetupInstallerUnit() (err error) {
	defer func() {
		if err != nil {
			log.Errorf("Failed to setup installer units: %s, reverting", err)
		}
	}()

	for _, unit := range installerUnits {
		if err = loadUnit(unit); err != nil {
			return err
		}
	}

	if err = systemdReload(); err != nil {
		return err
	}

	if err = enableUnit(installerUnit); err != nil {
		return err
	}

	if err = startUnit(installerUnit); err != nil {
		return err
	}
	return nil
}

// RemoveInstallerUnit removes the installer systemd units
func RemoveInstallerUnit() {
	var err error
	for _, unit := range installerUnits {
		if err = disableUnit(unit); err != nil {
			log.Warnf("Failed to disable %s: %s", unit, err)
		}
		if err = removeUnit(unit); err != nil {
			log.Warnf("Failed to stop %s: %s", unit, err)
		}
	}
}

// StartInstallerExperiment installs the experimental systemd units for the installer
func StartInstallerExperiment() error {
	return startUnit(installerUnitExp)
}

// StopInstallerExperiment installs the stable systemd units for the installer
func StopInstallerExperiment() error {
	return startUnit(installerUnit)
}
