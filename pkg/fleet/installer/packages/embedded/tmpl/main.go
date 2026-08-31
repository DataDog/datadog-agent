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
	for _, lay := range launchdEmbeddedLayouts {
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

// launchdTemplateData parameterises a launchd job definition.
//
// The stable and the -exp job sets are rendered from the same templates, which is what keeps
// them from drifting: a change to a job is one edit and both sets pick it up. They differ only
// in the fields below.
type launchdTemplateData struct {
	// LabelSuffix is appended to the job label and to its log file name: empty for the stable
	// set, "-exp" for the experiment set.
	LabelSuffix string
	// ProgramDir is the root the job's program is resolved under: the façade for the stable
	// set, the pool's experiment link for the -exp set.
	ProgramDir string
	// EtcDir is the configuration directory the job reads. launchd cannot supply a
	// configuration path at load time, so the definition names it itself.
	EtcDir string
	// FleetPoliciesDir is the Fleet-managed policy directory. Its trailing stable/experiment
	// segment is unrelated to the pool link of the same name and is the same for both sets:
	// an -exp job swaps the etc prefix and leaves that segment alone.
	FleetPoliciesDir string
	// Supervised reports whether launchd relaunches the job on an unsuccessful exit. The -exp
	// set omits KeepAlive entirely, which is what makes an experiment's exit terminal rather
	// than one iteration of a respawn loop.
	Supervised bool
	// Stable distinguishes the two sets for everything that is neither supervision nor a path,
	// such as the experiment-only environment variables.
	Stable bool

	// The remaining fields are the fixed state root. State is singular: one pidfile, one run
	// directory, one log directory, whichever job set is loaded.
	RunDir     string
	LogDir     string
	AgentUser  string
	AgentGroup string
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

func mustRenderLaunchdJob(name string, data launchdTemplateData) []byte {
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

// jobSetLaunchd renders both launchd job sets. The installer daemon has no -exp variant: it is
// the process that supervises an experiment, so it is never part of one.
func jobSetLaunchd(stableData, expData launchdTemplateData) map[string][]byte {
	return map[string][]byte{
		"com.datadoghq.installer.plist":      mustRenderLaunchdJob("com.datadoghq.installer.plist", stableData),
		"com.datadoghq.agent.plist":          mustRenderLaunchdJob("com.datadoghq.agent.plist", stableData),
		"com.datadoghq.agent-exp.plist":      mustRenderLaunchdJob("com.datadoghq.agent.plist", expData),
		"com.datadoghq.sysprobe.plist":       mustRenderLaunchdJob("com.datadoghq.sysprobe.plist", stableData),
		"com.datadoghq.sysprobe-exp.plist":   mustRenderLaunchdJob("com.datadoghq.sysprobe.plist", expData),
		"com.datadoghq.data-plane.plist":     mustRenderLaunchdJob("com.datadoghq.data-plane.plist", stableData),
		"com.datadoghq.data-plane-exp.plist": mustRenderLaunchdJob("com.datadoghq.data-plane.plist", expData),
	}
}

func unitSetSystemd(stableData, expData installerTemplateData, ambiantCapabilitiesSupported bool) map[string][]byte {
	units := map[string][]byte{
		"datadog-agent.service":                mustReadUnit("datadog-agent.service", stableData, ambiantCapabilitiesSupported, false),
		"datadog-agent-exp.service":            mustReadUnit("datadog-agent.service", expData, ambiantCapabilitiesSupported, false),
		"datadog-agent-installer.service":      mustReadUnit("datadog-agent-installer.service", stableData, ambiantCapabilitiesSupported, false),
		"datadog-agent-installer-exp.service":  mustReadUnit("datadog-agent-installer.service", expData, ambiantCapabilitiesSupported, false),
		"datadog-agent-data-plane.service":     mustReadUnit("datadog-agent-data-plane.service", stableData, ambiantCapabilitiesSupported, false),
		"datadog-agent-data-plane-exp.service": mustReadUnit("datadog-agent-data-plane.service", expData, ambiantCapabilitiesSupported, false),
		"datadog-agent-trace.service":          mustReadUnit("datadog-agent-trace.service", stableData, ambiantCapabilitiesSupported, false),
		"datadog-agent-trace-exp.service":      mustReadUnit("datadog-agent-trace.service", expData, ambiantCapabilitiesSupported, false),
		"datadog-agent-process.service":        mustReadUnit("datadog-agent-process.service", stableData, ambiantCapabilitiesSupported, false),
		"datadog-agent-process-exp.service":    mustReadUnit("datadog-agent-process.service", expData, ambiantCapabilitiesSupported, false),
		"datadog-agent-security.service":       mustReadUnit("datadog-agent-security.service", stableData, ambiantCapabilitiesSupported, false),
		"datadog-agent-security-exp.service":   mustReadUnit("datadog-agent-security.service", expData, ambiantCapabilitiesSupported, false),
		"datadog-agent-sysprobe.service":       mustReadUnit("datadog-agent-sysprobe.service", stableData, ambiantCapabilitiesSupported, false),
		"datadog-agent-sysprobe-exp.service":   mustReadUnit("datadog-agent-sysprobe.service", expData, ambiantCapabilitiesSupported, false),
		"datadog-agent-action.service":         mustReadUnit("datadog-agent-action.service", stableData, ambiantCapabilitiesSupported, false),
		"datadog-agent-action-exp.service":     mustReadUnit("datadog-agent-action.service", expData, ambiantCapabilitiesSupported, false),
		"datadog-agent-ddot.service":           mustReadUnit("datadog-agent-ddot.service", stableData, ambiantCapabilitiesSupported, false),
		"datadog-agent-ddot-exp.service":       mustReadUnit("datadog-agent-ddot.service", expData, ambiantCapabilitiesSupported, false),
	}
	return units
}

// For memory efficiency, procmgr units only defines the units that are different from the systemd units.
// Getting the procmgr units will fallback to the systemd units if not find.
func unitSetProcmgr(stableData, expData installerTemplateData, ambiantCapabilitiesSupported bool) map[string][]byte {
	units := map[string][]byte{
		"datadog-agent.service":             mustReadUnit("datadog-agent.service", stableData, ambiantCapabilitiesSupported, true),
		"datadog-agent-exp.service":         mustReadUnit("datadog-agent.service", expData, ambiantCapabilitiesSupported, true),
		"datadog-agent-procmgr.service":     mustReadUnit("datadog-agent-procmgr.service", stableData, ambiantCapabilitiesSupported, true),
		"datadog-agent-procmgr-exp.service": mustReadUnit("datadog-agent-procmgr.service", expData, ambiantCapabilitiesSupported, true),
	}
	return units
}

func yamlSet() map[string][]byte {
	return map[string][]byte{
		// The files are always the same, nothing to resolve from the template
		"datadog-agent-ddot.yaml":            mustRenderYAMLConfig("datadog-agent-ddot.yaml", installerTemplateData{}),
		"datadog-agent-action-executor.yaml": mustRenderYAMLConfig("datadog-agent-action-executor.yaml", installerTemplateData{}),
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
		{subdir: "sd/oci", units: unitSetSystemd(stableDataOCI, expDataOCI, true)},
		{subdir: "sd/debrpm", units: unitSetSystemd(stableDataDebRpm, expDataDebRpm, true)},
		{subdir: "sd/oci-nc", units: unitSetSystemd(stableDataOCI, expDataOCI, false)},
		{subdir: "sd/debrpm-nc", units: unitSetSystemd(stableDataDebRpm, expDataDebRpm, false)},
	}
	procmgrEmbeddedLayouts = []embeddedLayout{
		{subdir: "pm/oci", units: unitSetProcmgr(stableDataOCI, expDataOCI, true)},
		{subdir: "pm/debrpm", units: unitSetProcmgr(stableDataDebRpm, expDataDebRpm, true)},
		{subdir: "pm/oci-nc", units: unitSetProcmgr(stableDataOCI, expDataOCI, false)},
		{subdir: "pm/debrpm-nc", units: unitSetProcmgr(stableDataDebRpm, expDataDebRpm, false)},
		{subdir: "pm/processes.d", units: yamlSet()},
	}
	// macOS state is fixed and singular: /opt/datadog-agent holds etc, etc-exp, run and logs,
	// created once and preserved across every upgrade. Code is versioned and pooled under
	// /opt/datadog-packages, addressed only through a façade or through the experiment link.
	stableDataLaunchd = launchdTemplateData{
		LabelSuffix:      "",
		ProgramDir:       "/opt/datadog-agent",
		EtcDir:           "/opt/datadog-agent/etc",
		FleetPoliciesDir: "/opt/datadog-agent/etc/managed/datadog-agent/stable",
		Supervised:       true,
		Stable:           true,
		RunDir:           "/opt/datadog-agent/run",
		LogDir:           "/opt/datadog-agent/logs",
		AgentUser:        "_dd-agent",
		AgentGroup:       "daemon",
	}
	expDataLaunchd = launchdTemplateData{
		LabelSuffix:      "-exp",
		ProgramDir:       "/opt/datadog-packages/datadog-agent/experiment",
		EtcDir:           "/opt/datadog-agent/etc-exp",
		FleetPoliciesDir: "/opt/datadog-agent/etc-exp/managed/datadog-agent/stable",
		Supervised:       false,
		Stable:           false,
		RunDir:           "/opt/datadog-agent/run",
		LogDir:           "/opt/datadog-agent/logs",
		AgentUser:        "_dd-agent",
		AgentGroup:       "daemon",
	}

	windowsEmbeddedLayouts = []embeddedLayout{
		{subdir: "windows", units: windowsProcmgrYAMLFile("datadog-agent-ddot.yaml", "datadog-agent-ddot-windows.yaml", windowsDDOTCodegenData)},
		{subdir: "windows", units: windowsProcmgrYAMLFile("datadog-agent-data-plane.yaml", "datadog-agent-data-plane-windows.yaml", windowsADPCodegenData)},
		{subdir: "windows", units: windowsProcmgrYAMLFile("datadog-agent-action.yaml", "datadog-agent-action-windows.yaml", windowsPARCodegenData)},
		{subdir: "windows", units: windowsProcmgrYAMLFile("datadog-agent-action-executor.yaml", "datadog-agent-action-executor-windows.yaml", windowsPARCodegenData)},
	}
	launchdEmbeddedLayouts = []embeddedLayout{
		{subdir: "darwin", units: jobSetLaunchd(stableDataLaunchd, expDataLaunchd)},
	}
)
