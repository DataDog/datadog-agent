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
	for _, lay := range systemdEmbeddedLayouts {
		if err := lay.writeFilesToSubdir(outputDir); err != nil {
			return err
		}
	}
	for _, lay := range procmgrEmbeddedLayouts {
		if err := lay.writeFilesToSubdir(outputDir); err != nil {
			return err
		}
	}
	for _, lay := range windowsEmbeddedLayouts {
		if err := lay.writeFilesToSubdir(outputDir); err != nil {
			return err
		}
	}
	return nil
}

// fs is the embedded filesystem for the installer.
//
//go:embed *.tmpl
var embedded embed.FS

type installerTemplateData struct {
	InstallDir       string
	EtcDir           string
	FleetPoliciesDir string
	PIDDir           string
	Stable           bool
}

type templateData struct {
	installerTemplateData
	AmbiantCapabilitiesSupported bool
	Procmgr                      bool
}

type embeddedLayout struct {
	subdir string
	units  map[string][]byte
}

func (l embeddedLayout) writeFilesToSubdir(root string) error {
	subdirPath := filepath.Join(root, l.subdir)
	if err := os.MkdirAll(subdirPath, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", subdirPath, err)
	}
	for name, content := range l.units {
		path := filepath.Join(subdirPath, name)
		if err := os.WriteFile(path, content, 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", path, err)
		}
	}
	return nil
}

func mustRenderTemplate(name string, data installerTemplateData, ambiantCapabilitiesSupported bool, procmgr bool) []byte {
	tmpl, err := template.ParseFS(embedded, name)
	if err != nil {
		panic(err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, templateData{
		installerTemplateData:        data,
		AmbiantCapabilitiesSupported: ambiantCapabilitiesSupported,
		Procmgr:                      procmgr,
	}); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func mustReadUnit(name string, data installerTemplateData, ambiantCapabilitiesSupported bool, procmgr bool) []byte {
	return mustRenderTemplate(name+".tmpl", data, ambiantCapabilitiesSupported, procmgr)
}

func mustRenderYAMLConfig(name string, data installerTemplateData) []byte {
	return mustRenderTemplate(name+".tmpl", data, false, true)
}

func unitSet(stableData, expData installerTemplateData, ambiantCapabilitiesSupported bool, procmgr bool) map[string][]byte {
	units := map[string][]byte{
		"datadog-agent.service":                mustReadUnit("datadog-agent.service", stableData, ambiantCapabilitiesSupported, procmgr),
		"datadog-agent-exp.service":            mustReadUnit("datadog-agent.service", expData, ambiantCapabilitiesSupported, procmgr),
		"datadog-agent-installer.service":      mustReadUnit("datadog-agent-installer.service", stableData, ambiantCapabilitiesSupported, procmgr),
		"datadog-agent-installer-exp.service":  mustReadUnit("datadog-agent-installer.service", expData, ambiantCapabilitiesSupported, procmgr),
		"datadog-agent-data-plane.service":     mustReadUnit("datadog-agent-data-plane.service", stableData, ambiantCapabilitiesSupported, procmgr),
		"datadog-agent-data-plane-exp.service": mustReadUnit("datadog-agent-data-plane.service", expData, ambiantCapabilitiesSupported, procmgr),
		"datadog-agent-trace.service":          mustReadUnit("datadog-agent-trace.service", stableData, ambiantCapabilitiesSupported, procmgr),
		"datadog-agent-trace-exp.service":      mustReadUnit("datadog-agent-trace.service", expData, ambiantCapabilitiesSupported, procmgr),
		"datadog-agent-process.service":        mustReadUnit("datadog-agent-process.service", stableData, ambiantCapabilitiesSupported, procmgr),
		"datadog-agent-process-exp.service":    mustReadUnit("datadog-agent-process.service", expData, ambiantCapabilitiesSupported, procmgr),
		"datadog-agent-security.service":       mustReadUnit("datadog-agent-security.service", stableData, ambiantCapabilitiesSupported, procmgr),
		"datadog-agent-security-exp.service":   mustReadUnit("datadog-agent-security.service", expData, ambiantCapabilitiesSupported, procmgr),
		"datadog-agent-sysprobe.service":       mustReadUnit("datadog-agent-sysprobe.service", stableData, ambiantCapabilitiesSupported, procmgr),
		"datadog-agent-sysprobe-exp.service":   mustReadUnit("datadog-agent-sysprobe.service", expData, ambiantCapabilitiesSupported, procmgr),
		"datadog-agent-action.service":         mustReadUnit("datadog-agent-action.service", stableData, ambiantCapabilitiesSupported, procmgr),
		"datadog-agent-action-exp.service":     mustReadUnit("datadog-agent-action.service", expData, ambiantCapabilitiesSupported, procmgr),

		// Keep DDOT service unit event with procmgr for the deprecated datadog-agent-ddot package standalone (not managed by procmgr)
		// Can be added only for procmgr=false once we remove the standalone package
		"datadog-agent-ddot.service":     mustReadUnit("datadog-agent-ddot.service", stableData, ambiantCapabilitiesSupported, procmgr),
		"datadog-agent-ddot-exp.service": mustReadUnit("datadog-agent-ddot.service", expData, ambiantCapabilitiesSupported, procmgr),
	}
	if procmgr {
		units["datadog-agent-procmgr.service"] = mustReadUnit("datadog-agent-procmgr.service", stableData, ambiantCapabilitiesSupported, procmgr)
		units["datadog-agent-procmgr-exp.service"] = mustReadUnit("datadog-agent-procmgr.service", expData, ambiantCapabilitiesSupported, procmgr)
	}
	return units
}

func yamlSet(stableData, expData installerTemplateData) map[string][]byte {
	return map[string][]byte{
		"datadog-agent-ddot.yaml":     mustRenderYAMLConfig("datadog-agent-ddot.yaml", stableData),
		"datadog-agent-ddot-exp.yaml": mustRenderYAMLConfig("datadog-agent-ddot.yaml", expData),
	}
}

func windowsProcmgrYAMLFile(yamlFile, windowsFile string, codegen installerTemplateData) map[string][]byte {
	return map[string][]byte{
		yamlFile: mustRenderYAMLConfig(windowsFile, codegen),
	}
}

var (
	stableDataOCI = installerTemplateData{
		InstallDir:       "/opt/datadog-packages/datadog-agent/stable",
		EtcDir:           "/etc/datadog-agent",
		FleetPoliciesDir: "/etc/datadog-agent/managed/datadog-agent/stable",
		PIDDir:           "/opt/datadog-packages/datadog-agent/stable",
		Stable:           true,
	}
	expDataOCI = installerTemplateData{
		InstallDir:       "/opt/datadog-packages/datadog-agent/experiment",
		EtcDir:           "/etc/datadog-agent-exp",
		FleetPoliciesDir: "/etc/datadog-agent-exp/managed/datadog-agent/stable",
		PIDDir:           "/opt/datadog-packages/datadog-agent/experiment",
		Stable:           false,
	}
	stableDataDebRpm = installerTemplateData{
		InstallDir:       "/opt/datadog-agent",
		EtcDir:           "/etc/datadog-agent",
		FleetPoliciesDir: "/etc/datadog-agent/managed/datadog-agent/stable",
		PIDDir:           "/opt/datadog-agent",
		Stable:           true,
	}
	expDataDebRpm = installerTemplateData{
		InstallDir:       "/opt/datadog-agent",
		EtcDir:           "/etc/datadog-agent-exp",
		FleetPoliciesDir: "/etc/datadog-agent-exp/managed/datadog-agent/stable",
		PIDDir:           "/opt/datadog-agent",
		Stable:           false,
	}
	windowsDDOTCodegenData = installerTemplateData{
		InstallDir:       "__DDOT_INSTALL_ROOT__",
		EtcDir:           "__DDOT_ETC_ROOT__",
		FleetPoliciesDir: "__DDOT_FLEET_POLICIES_DIR__",
		PIDDir:           "",
		Stable:           true,
	}
	windowsADPCodegenData = installerTemplateData{
		InstallDir:       "__ADP_INSTALL_ROOT__",
		EtcDir:           "__ADP_ETC_ROOT__",
		FleetPoliciesDir: "__ADP_FLEET_POLICIES_DIR__",
		Stable:           true,
	}
	windowsPARCodegenData = installerTemplateData{
		InstallDir:       "__PAR_INSTALL_ROOT__",
		EtcDir:           "__PAR_ETC_ROOT__",
		FleetPoliciesDir: "__PAR_FLEET_POLICIES_DIR__",
		PIDDir:           "",
		Stable:           true,
	}

	// Ideally the folder names would be systemd and procmgr (instead of sd and pm)
	// and -nocap (instead of -nc)
	// but windows has a limit of file path length, so we use shorter names
	systemdEmbeddedLayouts = []embeddedLayout{
		{subdir: "sd/oci", units: unitSet(stableDataOCI, expDataOCI, true, false)},
		{subdir: "sd/debrpm", units: unitSet(stableDataDebRpm, expDataDebRpm, true, false)},
		{subdir: "sd/oci-nc", units: unitSet(stableDataOCI, expDataOCI, false, false)},
		{subdir: "sd/debrpm-nc", units: unitSet(stableDataDebRpm, expDataDebRpm, false, false)},
	}
	procmgrEmbeddedLayouts = []embeddedLayout{
		{subdir: "pm/oci", units: unitSet(stableDataOCI, expDataOCI, true, true)},
		{subdir: "pm/debrpm", units: unitSet(stableDataDebRpm, expDataDebRpm, true, true)},
		{subdir: "pm/oci-nc", units: unitSet(stableDataOCI, expDataOCI, false, true)},
		{subdir: "pm/debrpm-nc", units: unitSet(stableDataDebRpm, expDataDebRpm, false, true)},
		{subdir: "pm/oci", units: yamlSet(stableDataOCI, expDataOCI)},
		{subdir: "pm/debrpm", units: yamlSet(stableDataDebRpm, expDataDebRpm)},
		{subdir: "pm/oci-nc", units: yamlSet(stableDataOCI, expDataOCI)},
		{subdir: "pm/debrpm-nc", units: yamlSet(stableDataDebRpm, expDataDebRpm)},
	}
	windowsEmbeddedLayouts = []embeddedLayout{
		{subdir: "windows", units: windowsProcmgrYAMLFile("datadog-agent-ddot.yaml", "datadog-agent-ddot-windows.yaml", windowsDDOTCodegenData)},
		{subdir: "windows", units: windowsProcmgrYAMLFile("datadog-agent-data-plane.yaml", "datadog-agent-data-plane-windows.yaml", windowsADPCodegenData)},
		{subdir: "windows", units: windowsProcmgrYAMLFile("datadog-agent-action.yaml", "datadog-agent-action-windows.yaml", windowsPARCodegenData)},
	}
)
