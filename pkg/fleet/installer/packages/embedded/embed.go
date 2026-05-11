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

//go:embed tmpl/gen/debrpm/datadog-agent-ddot.yaml
//go:embed tmpl/gen/debrpm/datadog-agent-ddot-exp.yaml
//go:embed tmpl/gen/debrpm/datadog-agent-ddot-standalone.yaml
//go:embed tmpl/gen/debrpm/datadog-agent-ddot-standalone-exp.yaml
//go:embed tmpl/gen/debrpm-nocap/datadog-agent-ddot.yaml
//go:embed tmpl/gen/debrpm-nocap/datadog-agent-ddot-exp.yaml
//go:embed tmpl/gen/debrpm-nocap/datadog-agent-ddot-standalone.yaml
//go:embed tmpl/gen/debrpm-nocap/datadog-agent-ddot-standalone-exp.yaml
//go:embed tmpl/gen/oci/datadog-agent-ddot.yaml
//go:embed tmpl/gen/oci/datadog-agent-ddot-exp.yaml
//go:embed tmpl/gen/oci/datadog-agent-ddot-standalone.yaml
//go:embed tmpl/gen/oci/datadog-agent-ddot-standalone-exp.yaml
//go:embed tmpl/gen/oci-nocap/datadog-agent-ddot.yaml
//go:embed tmpl/gen/oci-nocap/datadog-agent-ddot-exp.yaml
//go:embed tmpl/gen/oci-nocap/datadog-agent-ddot-standalone.yaml
//go:embed tmpl/gen/oci-nocap/datadog-agent-ddot-standalone-exp.yaml
var ddotProcessYAML embed.FS

// GetDDOTProcessConfig returns the embedded DDOT process YAML bytes for the
// given systemd layout (OCI vs deb/rpm), stable vs experiment channel, and
// ambient capabilities support (same directory convention as GetSystemdUnit).
// When standalone is true, returns YAML for the datadog-agent-ddot package
// layout (embedded/bin, OCI paths under datadog-agent-ddot); when false, the
// DDOT extension layout (ext/ddot under the agent package).
func GetDDOTProcessConfig(unitType SystemdUnitType, stable bool, ambiantCapabilitiesSupported bool, standalone bool) ([]byte, error) {
	dir := string(unitType)
	if !ambiantCapabilitiesSupported {
		dir += "-nocap"
	}
	exp := ""
	if !stable {
		exp = "-exp"
	}
	prefix := "datadog-agent-ddot"
	if standalone {
		prefix += "-standalone"
	}
	name := prefix + exp + ".yaml"
	return ddotProcessYAML.ReadFile(filepath.Join("tmpl/gen", dir, name))
}

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
