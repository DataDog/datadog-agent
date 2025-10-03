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

// SystemdUnitType is the type of systemd unit.
type SystemdUnitType string

const (
	// SystemdUnitTypeOCI is the type of systemd unit for OCI.
	SystemdUnitTypeOCI SystemdUnitType = "oci"
	// SystemdUnitTypeDebRpm is the type of systemd unit for deb/rpm.
	SystemdUnitTypeDebRpm SystemdUnitType = "debrpm"
)

//go:embed templates/gen/sysvinit/*.erb
var sysvinitScripts embed.FS

// SysvinitScriptType is the type of sysvinit script.
type SysvinitScriptType string

const (
	// SysvinitScriptTypeAgent is the main datadog agent sysvinit script.
	SysvinitScriptTypeAgent SysvinitScriptType = "sysvinit_debian.erb"
	// SysvinitScriptTypeProcess is the process agent sysvinit script.
	SysvinitScriptTypeProcess SysvinitScriptType = "sysvinit_debian.process.erb"
	// SysvinitScriptTypeSecurity is the security agent sysvinit script.
	SysvinitScriptTypeSecurity SysvinitScriptType = "sysvinit_debian.security.erb"
	// SysvinitScriptTypeTrace is the trace agent sysvinit script.
	SysvinitScriptTypeTrace SysvinitScriptType = "sysvinit_debian.trace.erb"
)

// GetSystemdUnit returns the systemd unit for the given name.
func GetSystemdUnit(name string, unitType SystemdUnitType, ambiantCapabilitiesSupported bool) ([]byte, error) {
	dir := string(unitType)
	if !ambiantCapabilitiesSupported {
		dir += "-nocap"
	}
	return systemdUnits.ReadFile(filepath.Join("tmpl/gen", dir, name))
}

// GetSysvinitScript returns the sysvinit script for the given script type.
func GetSysvinitScript(scriptType SysvinitScriptType) ([]byte, error) {
	return sysvinitScripts.ReadFile(filepath.Join("templates/gen/sysvinit", string(scriptType)))
}
