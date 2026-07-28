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
	for _, lay := range windowsProcmgrLayouts {
		if err := lay.writeFilesToSubdir(outputDir); err != nil {
			return err
		}
	}
	for _, lay := range linuxProcmgrYAMLLayouts {
		if err := lay.writeFilesToSubdir(outputDir); err != nil {
			return err
		}
	}
	for _, lay := range systemdEmbeddedLayouts {
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
	InstallDir                   string
	EtcDir                       string
	FleetPoliciesDir             string
	PIDDir                       string
	Stable                       bool
	AmbiantCapabilitiesSupported bool
	// Procmgr selects the service manager the unit set belongs to. It drives which supervision
	// path datadog-agent.service pulls up: dd-procmgrd, or the per-payload systemd units.
	Procmgr bool
}

func (d installerTemplateData) withProcmgr() installerTemplateData {
	d.Procmgr = true
	return d
}

type templateData struct {
	installerTemplateData
	AmbiantCapabilitiesSupported bool
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

func mustRenderTemplate(name string, data installerTemplateData, ambiantCapabilitiesSupported bool) []byte {
	tmpl, err := template.ParseFS(embedded, name)
	if err != nil {
		panic(err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, templateData{
		installerTemplateData:        data,
		AmbiantCapabilitiesSupported: ambiantCapabilitiesSupported,
	}); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func mustReadSystemdUnit(name string, data installerTemplateData, ambiantCapabilitiesSupported bool) []byte {
	return mustRenderTemplate(name+".tmpl", data, ambiantCapabilitiesSupported)
}

func mustRenderYAMLConfig(name string, data installerTemplateData) []byte {
	return mustRenderTemplate(name+".tmpl", data, false)
}

// unitSet renders the units for one service manager. datadog-agent-ddot.service is present in both
// sets: under procmgr the Agent package no longer references it, but the standalone
// datadog-agent-ddot package still installs it.
func unitSet(stableData, expData installerTemplateData, ambiantCapabilitiesSupported bool) map[string][]byte {
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
	}
	if stableData.Procmgr {
		units["datadog-agent-procmgr.service"] = mustReadSystemdUnit("datadog-agent-procmgr.service", stableData, ambiantCapabilitiesSupported)
		units["datadog-agent-procmgr-exp.service"] = mustReadSystemdUnit("datadog-agent-procmgr.service", expData, ambiantCapabilitiesSupported)
	}
	return units
}

// procmgrConfig renders a processes.d entry. The file name is the same for stable and experiment —
// the two live in different install trees, and dd-procmgrd resolves ${DD_CONF_DIR} per daemon.
func procmgrConfig(data installerTemplateData) map[string][]byte {
	return map[string][]byte{
		"datadog-agent-ddot.yaml": mustRenderYAMLConfig("datadog-agent-ddot.yaml", data),
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
	windowsProcmgrLayouts = []embeddedLayout{
		{subdir: "windows", units: windowsProcmgrYAMLFile("datadog-agent-ddot.yaml", "datadog-agent-ddot-windows.yaml", windowsDDOTCodegenData)},
		{subdir: "windows", units: windowsProcmgrYAMLFile("datadog-agent-data-plane.yaml", "datadog-agent-data-plane-windows.yaml", windowsADPCodegenData)},
		{subdir: "windows", units: windowsProcmgrYAMLFile("datadog-agent-action.yaml", "datadog-agent-action-windows.yaml", windowsPARCodegenData)},
	}
	// linuxProcmgrYAMLLayouts mirror the install layout: the processes.d entries sit in the
	// subdirectory dd-procmgrd reads. Ambient capabilities are a systemd concept, so the -nocap
	// flavors carry no configs.
	linuxProcmgrYAMLLayouts = []embeddedLayout{
		{subdir: "procmgr/oci/processes.d", units: procmgrConfig(stableDataOCI.withProcmgr())},
		{subdir: "procmgr/oci/processes-exp.d", units: procmgrConfig(expDataOCI.withProcmgr())},
		{subdir: "procmgr/debrpm/processes.d", units: procmgrConfig(stableDataDebRpm.withProcmgr())},
		{subdir: "procmgr/debrpm/processes-exp.d", units: procmgrConfig(expDataDebRpm.withProcmgr())},
	}
	systemdEmbeddedLayouts = []embeddedLayout{
		{subdir: "systemd/oci", units: unitSet(stableDataOCI, expDataOCI, true)},
		{subdir: "systemd/debrpm", units: unitSet(stableDataDebRpm, expDataDebRpm, true)},
		{subdir: "systemd/oci-nocap", units: unitSet(stableDataOCI, expDataOCI, false)},
		{subdir: "systemd/debrpm-nocap", units: unitSet(stableDataDebRpm, expDataDebRpm, false)},

		{subdir: "procmgr/oci", units: unitSet(stableDataOCI.withProcmgr(), expDataOCI.withProcmgr(), true)},
		{subdir: "procmgr/debrpm", units: unitSet(stableDataDebRpm.withProcmgr(), expDataDebRpm.withProcmgr(), true)},
		{subdir: "procmgr/oci-nocap", units: unitSet(stableDataOCI.withProcmgr(), expDataOCI.withProcmgr(), false)},
		{subdir: "procmgr/debrpm-nocap", units: unitSet(stableDataDebRpm.withProcmgr(), expDataDebRpm.withProcmgr(), false)},
	}
)
