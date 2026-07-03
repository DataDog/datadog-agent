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

//go:embed tmpl/gen/oci/*.service
//go:embed tmpl/gen/debrpm/*.service
var systemdUnits embed.FS

// DDOTProcessConfig is the rendered process manager config for DDOT (deb/rpm layout). Its
// --config/--core-config reference ${DD_CONF_DIR}, which the supervising dd-procmgr substitutes at
// launch with its config directory (stable or experiment).
//
//go:embed tmpl/gen/debrpm/datadog-agent-ddot.yaml
var DDOTProcessConfig string

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

// ProcessWindowsProcmgrConfig is the codegen-rendered process manager config for process-agent
// on Windows (see embedded/tmpl/main.go). Install time replaces __PROCESS_*__ placeholders.
//
//go:embed tmpl/gen/windows/datadog-agent-process.yaml
var ProcessWindowsProcmgrConfig string

// SystemdUnitType is the type of systemd unit.
type SystemdUnitType string

const (
	// SystemdUnitTypeOCI is the type of systemd unit for OCI.
	SystemdUnitTypeOCI SystemdUnitType = "oci"
	// SystemdUnitTypeDebRpm is the type of systemd unit for deb/rpm.
	SystemdUnitTypeDebRpm SystemdUnitType = "debrpm"
)

// GetSystemdUnit returns the systemd unit for the given name.
func GetSystemdUnit(name string, unitType SystemdUnitType, ambiantCapabilitiesSupported bool) ([]byte, error) {
	dir := string(unitType)
	if !ambiantCapabilitiesSupported {
		dir += "-nocap"
	}
	return systemdUnits.ReadFile(filepath.Join("tmpl/gen", dir, name))
}
