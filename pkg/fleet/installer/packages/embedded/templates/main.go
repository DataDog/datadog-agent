// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package main generates the templates for the installer.
package main

import (
	"bytes"
	"embed"
	"fmt"
	"log"
	"os"
	"path/filepath"

	template "github.com/DataDog/datadog-agent/pkg/template/text"
)

//go:generate go run ./main.go ./gen

func main() {
	if len(os.Args) != 2 {
		fmt.Printf("Usage: %s <output-dir>\n", os.Args[0])
		os.Exit(1)
	}
	outputDir := os.Args[1]

	if err := generate(outputDir); err != nil {
		log.Fatalf("Failed to generate templates: %v", err)
	}
}

func generate(outputDir string) error {
	err := os.MkdirAll(filepath.Join(outputDir, "oci"), 0755)
	if err != nil {
		return fmt.Errorf("failed to create directory for oci: %w", err)
	}
	err = os.MkdirAll(filepath.Join(outputDir, "debrpm"), 0755)
	if err != nil {
		return fmt.Errorf("failed to create directory for deb-rpm: %w", err)
	}
	for unit, content := range systemdUnitsOCI {
		filePath := filepath.Join(outputDir, "oci", unit)
		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			return fmt.Errorf("failed to create directory for %s: %w", unit, err)
		}
		if err := os.WriteFile(filePath, content, 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", unit, err)
		}
	}
	for unit, content := range systemdUnitsDebRpm {
		filePath := filepath.Join(outputDir, "debrpm", unit)
		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			return fmt.Errorf("failed to create directory for %s: %w", unit, err)
		}
		if err := os.WriteFile(filePath, content, 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", unit, err)
		}
	}
	return nil
}

// fs is the embedded filesystem for the installer.
//
//go:embed *.tmpl
var embedded embed.FS

type systemdTemplateData struct {
	InstallDir       string
	EtcDir           string
	FleetPoliciesDir string
	PIDDir           string
	Stable           bool
}

func mustReadSystemdUnit(name string, data systemdTemplateData) []byte {
	tmpl, err := template.ParseFS(embedded, name+".tmpl")
	if err != nil {
		panic(err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func systemdUnits(stableData, expData, ddotStableData, ddotExpData systemdTemplateData, includeInstaller bool) map[string][]byte {
	units := map[string][]byte{
		"datadog-agent.service":               mustReadSystemdUnit("datadog-agent.service", stableData),
		"datadog-agent-exp.service":           mustReadSystemdUnit("datadog-agent.service", expData),
		"datadog-agent-installer.service":     mustReadSystemdUnit("datadog-agent-installer.service", stableData),
		"datadog-agent-installer-exp.service": mustReadSystemdUnit("datadog-agent-installer.service", expData),
		"datadog-agent-trace.service":         mustReadSystemdUnit("datadog-agent-trace.service", stableData),
		"datadog-agent-trace-exp.service":     mustReadSystemdUnit("datadog-agent-trace.service", expData),
		"datadog-agent-process.service":       mustReadSystemdUnit("datadog-agent-process.service", stableData),
		"datadog-agent-process-exp.service":   mustReadSystemdUnit("datadog-agent-process.service", expData),
		"datadog-agent-security.service":      mustReadSystemdUnit("datadog-agent-security.service", stableData),
		"datadog-agent-security-exp.service":  mustReadSystemdUnit("datadog-agent-security.service", expData),
		"datadog-agent-sysprobe.service":      mustReadSystemdUnit("datadog-agent-sysprobe.service", stableData),
		"datadog-agent-sysprobe-exp.service":  mustReadSystemdUnit("datadog-agent-sysprobe.service", expData),
		"datadog-agent-ddot.service":          mustReadSystemdUnit("datadog-agent-ddot.service", ddotStableData),
		"datadog-agent-ddot-exp.service":      mustReadSystemdUnit("datadog-agent-ddot.service", ddotExpData),
	}
	if includeInstaller {
		units["datadog-installer.service"] = mustReadSystemdUnit("datadog-installer.service", stableData)
		units["datadog-installer-exp.service"] = mustReadSystemdUnit("datadog-installer.service", expData)
	}
	return units
}

var (
	stableDataOCI = systemdTemplateData{
		InstallDir:       "/opt/datadog-packages/datadog-agent/stable",
		EtcDir:           "/etc/datadog-agent",
		FleetPoliciesDir: "/etc/datadog-agent/managed/datadog-agent/stable",
		PIDDir:           "/opt/datadog-packages/datadog-agent/stable",
		Stable:           true,
	}
	expDataOCI = systemdTemplateData{
		InstallDir:       "/opt/datadog-packages/datadog-agent/experiment",
		EtcDir:           "/etc/datadog-agent",
		FleetPoliciesDir: "/etc/datadog-agent/managed/datadog-agent/experiment",
		PIDDir:           "/opt/datadog-packages/datadog-agent/experiment",
		Stable:           false,
	}

	stableDataDebRpm = systemdTemplateData{
		InstallDir:       "/opt/datadog-agent",
		EtcDir:           "/etc/datadog-agent",
		FleetPoliciesDir: "/etc/datadog-agent/managed/datadog-agent/stable",
		PIDDir:           "/opt/datadog-agent",
		Stable:           true,
	}
	expDataDebRpm = systemdTemplateData{
		InstallDir:       "/opt/datadog-agent",
		EtcDir:           "/etc/datadog-agent",
		FleetPoliciesDir: "/etc/datadog-agent/managed/datadog-agent/experiment",
		PIDDir:           "/opt/datadog-agent",
		Stable:           false,
	}

	ddotStableDataOCI = systemdTemplateData{
		InstallDir:       "/opt/datadog-packages/datadog-agent-ddot/stable",
		EtcDir:           "/etc/datadog-agent",
		FleetPoliciesDir: "/etc/datadog-agent/managed/datadog-agent/stable",
		PIDDir:           "/opt/datadog-packages/datadog-agent/stable",
		Stable:           true,
	}
	ddotExpDataOCI = systemdTemplateData{
		InstallDir:       "/opt/datadog-packages/datadog-agent-ddot/experiment",
		EtcDir:           "/etc/datadog-agent",
		FleetPoliciesDir: "/etc/datadog-agent/managed/datadog-agent/experiment",
		PIDDir:           "/opt/datadog-packages/datadog-agent/experiment",
		Stable:           false,
	}

	systemdUnitsOCI    = systemdUnits(stableDataOCI, expDataOCI, ddotStableDataOCI, ddotExpDataOCI, true)
	systemdUnitsDebRpm = systemdUnits(stableDataDebRpm, expDataDebRpm, stableDataDebRpm, expDataDebRpm, false)
)
