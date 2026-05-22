// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package embedded provides embedded files for the installer.
package embedded

import (
	"embed"
	"fmt"
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

//go:embed tmpl/gen/debrpm/datadog-agent-ddot-sa.yaml
//go:embed tmpl/gen/debrpm/datadog-agent-ddot-sa-exp.yaml
//go:embed tmpl/gen/debrpm-nocap/datadog-agent-ddot-sa.yaml
//go:embed tmpl/gen/debrpm-nocap/datadog-agent-ddot-sa-exp.yaml
//go:embed tmpl/gen/oci/datadog-agent-ddot.yaml
//go:embed tmpl/gen/oci/datadog-agent-ddot-exp.yaml
//go:embed tmpl/gen/oci/datadog-agent-ddot-sa.yaml
//go:embed tmpl/gen/oci/datadog-agent-ddot-sa-exp.yaml
//go:embed tmpl/gen/oci-nocap/datadog-agent-ddot.yaml
//go:embed tmpl/gen/oci-nocap/datadog-agent-ddot-exp.yaml
//go:embed tmpl/gen/oci-nocap/datadog-agent-ddot-sa.yaml
//go:embed tmpl/gen/oci-nocap/datadog-agent-ddot-sa-exp.yaml
var ddotProcessYAML embed.FS

// GetDDOTProcessConfig returns embedded DDOT process YAML (extension or standalone layout).
func GetDDOTProcessConfig(unitType SystemdUnitType, stable bool, ambiantCapabilitiesSupported bool, standalone bool) ([]byte, error) {
	if !standalone && unitType != SystemdUnitTypeOCI {
		return nil, fmt.Errorf("ddot extension procmgr yaml is OCI-only, got %s", unitType)
	}
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
		prefix += "-sa"
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
