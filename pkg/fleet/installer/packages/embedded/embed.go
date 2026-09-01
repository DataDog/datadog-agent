// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package embedded provides embedded files for the installer.
package embedded

import (
	"embed"
	"errors"
	"io/fs"
	"path"
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

// systemdUnits holds the unit set for the systemd service manager
//
//go:embed tmpl/gen/sd
var systemdUnits embed.FS

// procmgrUnits holds the unit set for the procmgr service manager: the .service for systemd units
// plus the .yaml processes.d entries it supervises.
//
//go:embed tmpl/gen/pm
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

// PARExecutorWindowsProcmgrConfig is the codegen-rendered process manager config for the PAR
// on-demand executor on Windows (see embedded/tmpl/main.go).
//
//go:embed tmpl/gen/windows/datadog-agent-action-executor.yaml
var PARExecutorWindowsProcmgrConfig string

// PARControlWindowsProcmgrConfig is the codegen-rendered process manager config for the PAR
// control plane on Windows (see embedded/tmpl/main.go). Installer replaces __PAR_*__
// placeholders.
//
//go:embed tmpl/gen/windows/datadog-agent-par-control.yaml
var PARControlWindowsProcmgrConfig string

// UnitType is the type of systemd unit.
type UnitType string

const (
	// UnitTypeOCI is the type of systemd unit for OCI.
	UnitTypeOCI UnitType = "oci"
	// UnitTypeDebRpm is the type of systemd unit for deb/rpm.
	UnitTypeDebRpm UnitType = "debrpm"
)

// GetSystemdUnit returns the unit for the given name, for the plain systemd service manager.
func GetSystemdUnit(name string, unitType UnitType, ambiantCapabilitiesSupported bool) ([]byte, error) {
	return systemdUnits.ReadFile(path.Join("tmpl/gen/sd", flavorDir(unitType, ambiantCapabilitiesSupported), name))
}

// GetProcmgrUnit returns the unit for the given name, for the procmgr service manager.
func GetProcmgrUnit(name string, unitType UnitType, ambiantCapabilitiesSupported bool) ([]byte, error) {
	data, err := procmgrUnits.ReadFile(path.Join("tmpl/gen/pm", flavorDir(unitType, ambiantCapabilitiesSupported), name))
	if errors.Is(err, fs.ErrNotExist) {
		return GetSystemdUnit(name, unitType, ambiantCapabilitiesSupported)
	}
	return data, err
}

// GetProcmgrProcess returns the process config for the given name (actually only for procmgr)
func GetProcmgrProcess(name string) ([]byte, error) {
	return procmgrUnits.ReadFile(path.Join("tmpl/gen/pm", "processes.d", name))
}

func flavorDir(unitType UnitType, ambiantCapabilitiesSupported bool) string {
	if ambiantCapabilitiesSupported {
		return string(unitType)
	}
	return string(unitType) + "-nc"
}
