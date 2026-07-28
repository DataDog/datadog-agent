// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package embedded provides embedded files for the installer.
package embedded

import (
	"embed"
	"path/filepath"
)

// ScriptDDCleanup is the embedded dd-cleanup script.
//
//go:embed scripts/dd-cleanup
var ScriptDDCleanup []byte

// ScriptDDContainerInstall is the embedded dd-container-install script.
//
//go:embed scripts/dd-container-install
var ScriptDDContainerInstall []byte

// ScriptDDHostInstall is the embedded dd-host-install script.
//
//go:embed scripts/dd-host-install
var ScriptDDHostInstall []byte

// systemdUnits holds the unit set for the plain systemd service manager: one unit per payload, no
// dd-procmgrd.
//
//go:embed tmpl/gen/systemd
var systemdUnits embed.FS

// procmgrUnits holds the unit set for the procmgr service manager: dd-procmgrd's own units plus the
// processes.d entries it supervises, and no unit for the payloads it took over.
//
//go:embed tmpl/gen/procmgr
var procmgrUnits embed.FS

// DDOTWindowsProcmgrConfig is the codegen-rendered process manager config for DDOT on Windows
// (see embedded/tmpl/main.go). Install time replaces __DDOT_*__ placeholders.
//
//go:embed tmpl/gen/windows/datadog-agent-ddot.yaml
var DDOTWindowsProcmgrConfig string

// ADPWindowsProcmgrConfig is the codegen-rendered process manager config for ADP on Windows
// (see embedded/tmpl/main.go). Install time replaces __ADP_*__ placeholders.
//
//go:embed tmpl/gen/windows/datadog-agent-data-plane.yaml
var ADPWindowsProcmgrConfig string

// PARWindowsProcmgrConfig is the codegen-rendered process manager config for PAR on Windows
// (see embedded/tmpl/main.go). Install time replaces __PAR_*__ placeholders.
//
//go:embed tmpl/gen/windows/datadog-agent-action.yaml
var PARWindowsProcmgrConfig string

// SystemdUnitType is the type of systemd unit.
type SystemdUnitType string

const (
	// SystemdUnitTypeOCI is the type of systemd unit for OCI.
	SystemdUnitTypeOCI SystemdUnitType = "oci"
	// SystemdUnitTypeDebRpm is the type of systemd unit for deb/rpm.
	SystemdUnitTypeDebRpm SystemdUnitType = "debrpm"
)

// GetSystemdUnit returns the unit for the given name, for the plain systemd service manager.
func GetSystemdUnit(name string, unitType SystemdUnitType, ambiantCapabilitiesSupported bool) ([]byte, error) {
	return systemdUnits.ReadFile(filepath.Join("tmpl/gen/systemd", flavorDir(unitType, ambiantCapabilitiesSupported), name))
}

// GetProcmgrUnit returns the unit for the given name, for the procmgr service manager.
func GetProcmgrUnit(name string, unitType SystemdUnitType, ambiantCapabilitiesSupported bool) ([]byte, error) {
	return procmgrUnits.ReadFile(filepath.Join("tmpl/gen/procmgr", flavorDir(unitType, ambiantCapabilitiesSupported), name))
}

// GetProcmgrConfig returns the processes.d entry for the given name. The name is the on-disk file
// name, identical for stable and experiment: the two are told apart by the install tree they are
// written to, so the variant is selected here instead.
func GetProcmgrConfig(name string, unitType SystemdUnitType, experiment bool) ([]byte, error) {
	processesDir := "processes.d"
	if experiment {
		processesDir = "processes-exp.d"
	}
	return procmgrUnits.ReadFile(filepath.Join("tmpl/gen/procmgr", string(unitType), processesDir, name))
}

func flavorDir(unitType SystemdUnitType, ambiantCapabilitiesSupported bool) string {
	if ambiantCapabilitiesSupported {
		return string(unitType)
	}
	return string(unitType) + "-nocap"
}
