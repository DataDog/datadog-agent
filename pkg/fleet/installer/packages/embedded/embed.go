// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package embedded provides embedded files for the installer.
package embedded

import (
	"embed"
	"path/filepath"
)

// ScriptDDCleanup is the embedded dd-cleanup script.
//
//go:embed scripts/linux/dd-cleanup
var ScriptDDCleanup []byte

// ScriptDDContainerInstall is the embedded dd-container-install script.
//
//go:embed scripts/linux/dd-container-install
var ScriptDDContainerInstall []byte

// ScriptDDHostInstall is the embedded dd-host-install script.
//
//go:embed scripts/linux/dd-host-install
var ScriptDDHostInstall []byte

//go:embed templates/gen/oci/*.service
//go:embed templates/gen/debrpm/*.service
var systemdUnits embed.FS

// SystemdUnitType is the type of systemd unit.
type SystemdUnitType string

const (
	// SystemdUnitTypeOCI is the type of systemd unit for OCI.
	SystemdUnitTypeOCI SystemdUnitType = "oci"
	// SystemdUnitTypeDebRpm is the type of systemd unit for deb/rpm.
	SystemdUnitTypeDebRpm SystemdUnitType = "debrpm"
)

// GetSystemdUnit returns the systemd unit for the given name.
func GetSystemdUnit(name string, unitType SystemdUnitType) ([]byte, error) {
	return systemdUnits.ReadFile(filepath.Join("templates/gen", string(unitType), name))
}
