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
	for unit, content := range systemdUnitsOCILegacyKernel {
		filePath := filepath.Join(outputDir, "oci-nocap", unit)
		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			return fmt.Errorf("failed to create directory for %s: %w", unit, err)
		}
		if err := os.WriteFile(filePath, content, 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", unit, err)
		}
	}
	for unit, content := range systemdUnitsDebRpmLegacyKernel {
		filePath := filepath.Join(outputDir, "debrpm-nocap", unit)
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
	InstallDir                   string
	EtcDir                       string
	FleetPoliciesDir             string
	PIDDir                       string
	Stable                       bool
	AmbiantCapabilitiesSupported bool
}

type templateData struct {
	systemdTemplateData
	AmbiantCapabilitiesSupported bool
}

func mustReadSystemdUnit(name string, data systemdTemplateData, ambiantCapabilitiesSupported bool) []byte {
	tmpl, err := template.ParseFS(embedded, name+".tmpl")
	if err != nil {
		panic(err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, templateData{
		systemdTemplateData:          data,
		AmbiantCapabilitiesSupported: ambiantCapabilitiesSupported,
	}); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func systemdUnits(stableData, expData systemdTemplateData, ambiantCapabilitiesSupported bool) map[string][]byte {
	units := map[string][]byte{
		"datadog-agent.service":                mustReadSystemdUnit("datadog-agent.service", stableData, ambiantCapabilitiesSupported),
		"datadog-agent-exp.service":            mustReadSystemdUnit("datadog-agent.service", expData, ambiantCapabilitiesSupported),
		"datadog-agent-installer.service":      mustReadSystemdUnit("datadog-agent-installer.service", stableData, ambiantCapabilitiesSupported),
		"datadog-agent-installer-exp.service":  mustReadSystemdUnit("datadog-agent-installer.service", expData, ambiantCapabilitiesSupported),
		"datadog-agent-data-plane.service":     mustReadSystemdUnit("datadog-agent-data-plane.service", stableData, ambiantCapabilitiesSupported),
		"datadog-agent-data-plane-exp.service": mustReadSystemdUnit("datadog-agent-data-plane.service", expData, ambiantCapabilitiesSupported),
		"datadog-agent-trace.service":          mustReadSystemdUnit("datadog-agent-trace.service", stableData, ambiantCapabilitiesSupported),
		"datadog-agent-trace-exp.service":      mustReadSystemdUnit("datadog-agent-trace.service", expData, ambiantCapabilitiesSupported),
		"datadog-agent-process.service":        mustReadSystemdUnit("datadog-agent-process.service", stableData, ambiantCapabilitiesSupported),
		"datadog-agent-process-exp.service":    mustReadSystemdUnit("datadog-agent-process.service", expData, ambiantCapabilitiesSupported),
		"datadog-agent-security.service":       mustReadSystemdUnit("datadog-agent-security.service", stableData, ambiantCapabilitiesSupported),
		"datadog-agent-security-exp.service":   mustReadSystemdUnit("datadog-agent-security.service", expData, ambiantCapabilitiesSupported),
		"datadog-agent-sysprobe.service":       mustReadSystemdUnit("datadog-agent-sysprobe.service", stableData, ambiantCapabilitiesSupported),
		"datadog-agent-sysprobe-exp.service":   mustReadSystemdUnit("datadog-agent-sysprobe.service", expData, ambiantCapabilitiesSupported),
		"datadog-agent-ddot.service":           mustReadSystemdUnit("datadog-agent-ddot.service", stableData, ambiantCapabilitiesSupported),
		"datadog-agent-ddot-exp.service":       mustReadSystemdUnit("datadog-agent-ddot.service", expData, ambiantCapabilitiesSupported),
		"datadog-agent-action.service":         mustReadSystemdUnit("datadog-agent-action.service", stableData, ambiantCapabilitiesSupported),
		"datadog-agent-action-exp.service":     mustReadSystemdUnit("datadog-agent-action.service", expData, ambiantCapabilitiesSupported),
		"datadog-agent-procmgrd.service":       mustReadSystemdUnit("datadog-agent-procmgrd.service", stableData, ambiantCapabilitiesSupported),
		"datadog-agent-procmgrd-exp.service":   mustReadSystemdUnit("datadog-agent-procmgrd.service", expData, ambiantCapabilitiesSupported),
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
		EtcDir:           "/etc/datadog-agent-exp",
		FleetPoliciesDir: "/etc/datadog-agent-exp/managed/datadog-agent/stable",
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
		EtcDir:           "/etc/datadog-agent-exp",
		FleetPoliciesDir: "/etc/datadog-agent-exp/managed/datadog-agent/stable",
		PIDDir:           "/opt/datadog-agent",
		Stable:           false,
	}

	systemdUnitsOCI    = systemdUnits(stableDataOCI, expDataOCI, true)
	systemdUnitsDebRpm = systemdUnits(stableDataDebRpm, expDataDebRpm, true)

	systemdUnitsOCILegacyKernel    = systemdUnits(stableDataOCI, expDataOCI, false)
	systemdUnitsDebRpmLegacyKernel = systemdUnits(stableDataDebRpm, expDataDebRpm, false)
)
