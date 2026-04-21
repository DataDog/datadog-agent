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

//go:embed tmpl/gen/oci/datadog-agent-ddot.yaml
//go:embed tmpl/gen/oci/datadog-agent-ddot-exp.yaml
//go:embed tmpl/gen/oci/datadog-agent-ddot-sa.yaml
//go:embed tmpl/gen/oci/datadog-agent-ddot-sa-exp.yaml
//go:embed tmpl/gen/debrpm/datadog-agent-ddot.yaml
//go:embed tmpl/gen/debrpm/datadog-agent-ddot-exp.yaml
//go:embed tmpl/gen/debrpm/datadog-agent-ddot-sa.yaml
//go:embed tmpl/gen/debrpm/datadog-agent-ddot-sa-exp.yaml
var processConfigs embed.FS

// DDOTProcessConfig is the rendered process manager config for DDOT (deb/rpm stable layout).
// Kept for backward compatibility; prefer GetDDOTProcessConfig for new code.
//
//go:embed tmpl/gen/debrpm/datadog-agent-ddot.yaml
var DDOTProcessConfig string

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

// GetDDOTProcessConfig returns the process manager YAML config for DDOT.
// When standalone is true, the returned config uses standalone DDOT package
// paths; when false, it uses the extension layout (binary under /ext/ddot).
// The nocap distinction does not apply to process configs (only systemd units
// use ambient capabilities), so the same file serves both cases.
func GetDDOTProcessConfig(unitType SystemdUnitType, stable bool, standalone bool) (string, error) {
	name := "datadog-agent-ddot"
	if standalone {
		name += "-sa"
	}
	if !stable {
		name += "-exp"
	}
	name += ".yaml"
	data, err := processConfigs.ReadFile(filepath.Join("tmpl/gen", string(unitType), name))
	if err != nil {
		return "", err
	}
	return string(data), nil
}
