// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package service provides a way to interact with os services
package service

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/installinfo"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/google/uuid"
	"gopkg.in/yaml.v2"
)

const (
	agentUnit         = "datadog-agent.service"
	traceAgentUnit    = "datadog-agent-trace.service"
	processAgentUnit  = "datadog-agent-process.service"
	systemProbeUnit   = "datadog-agent-sysprobe.service"
	securityAgentUnit = "datadog-agent-security.service"
	agentExp          = "datadog-agent-exp.service"
	traceAgentExp     = "datadog-agent-trace-exp.service"
	processAgentExp   = "datadog-agent-process-exp.service"
	systemProbeExp    = "datadog-agent-sysprobe-exp.service"
	securityAgentExp  = "datadog-agent-security-exp.service"
)

var (
	configDir       = "/etc/datadog-agent"
	installInfoFile = filepath.Join(configDir, "install_info")
	installSigFile  = filepath.Join(configDir, "install.json")
	stableUnits     = []string{
		agentUnit,
		traceAgentUnit,
		processAgentUnit,
		systemProbeUnit,
		securityAgentUnit,
	}
	experimentalUnits = []string{
		agentExp,
		traceAgentExp,
		processAgentExp,
		systemProbeExp,
		securityAgentExp,
	}
)

// SetupAgentUnits installs and starts the agent units
func SetupAgentUnits() (err error) {
	defer func() {
		if err != nil {
			log.Errorf("Failed to setup agent units: %s, reverting", err)
			RemoveAgentUnits()
		}
	}()

	for _, unit := range stableUnits {
		if err = loadUnit(unit); err != nil {
			return
		}
	}
	for _, unit := range experimentalUnits {
		if err = loadUnit(unit); err != nil {
			return
		}
	}

	if err = systemdReload(); err != nil {
		return
	}

	for _, unit := range stableUnits {
		if err = enableUnit(unit); err != nil {
			return
		}
	}
	for _, unit := range stableUnits {
		if err = startUnit(unit); err != nil {
			return
		}
	}
	setInstallInfo()
	return
}

// RemoveAgentUnits stops and removes the agent units
func RemoveAgentUnits() {
	// stop experiments, they can restart stable agent
	for _, unit := range experimentalUnits {
		if err := stopUnit(unit); err != nil {
			log.Warnf("Failed to stop %s: %s", unit, err)
		}
	}
	// stop stable agents
	for _, unit := range stableUnits {
		if err := stopUnit(unit); err != nil {
			log.Warnf("Failed to stop %s: %s", unit, err)
		}
	}
	// purge experimental units
	for _, unit := range experimentalUnits {
		if err := disableUnit(unit); err != nil {
			log.Warnf("Failed to disable %s: %s", unit, err)
		}
		if err := removeUnit(unit); err != nil {
			log.Warnf("Failed to remove %s: %s", unit, err)
		}
	}
	// purge stable units
	for _, unit := range stableUnits {
		if err := disableUnit(unit); err != nil {
			log.Warnf("Failed to disable %s: %s", unit, err)
		}
		if err := removeUnit(unit); err != nil {
			log.Warnf("Failed to remove %s: %s", unit, err)
		}
	}
	cleanInstallInfo()
}

// StartAgentExperiment starts the agent experiment
func StartAgentExperiment() error {
	return startUnit(agentExp)
}

// StopAgentExperiment stops the agent experiment
func StopAgentExperiment() error {
	return startUnit(agentUnit)
}

func setInstallInfo() {
	// avoid rewriting the files if they already exist
	if _, err := os.Stat(installInfoFile); err == nil {
		log.Info("Install info file already exists, skipping")
		return
	}
	tool, version := getToolVersion()
	if err := writeInstallInfo(tool, version); err != nil {
		log.Warnf("Failed to write install info: %s", err)
	}
	if err := writeInstallSignature(); err != nil {
		log.Warnf("Failed to write install signature: %s", err)
	}
}

func cleanInstallInfo() {
	if err := os.Remove(installInfoFile); err != nil && !os.IsNotExist(err) {
		log.Warnf("Failed to remove install info file: %s", err)
	}
	if err := os.Remove(installSigFile); err != nil && !os.IsNotExist(err) {
		log.Warnf("Failed to remove install signature file: %s", err)
	}
}

func getToolVersion() (string, string) {
	tool := "unknown"
	version := "unknown"
	if _, err := exec.LookPath("dpkg-query"); err == nil {
		tool = "dpkg"
		toolVersion, err := getDpkgVersion()
		if err == nil {
			version = toolVersion
		}
	}
	return tool, version
}

func getDpkgVersion() (string, error) {
	cmd := exec.Command("dpkg-query", "--showformat=${Version}", "--show", "dpkg")
	output, err := cmd.Output()
	if err != nil {
		log.Warnf("Failed to get dpkg version: %s", err)
		return "", err
	}
	splitVersion := strings.Split(strings.TrimSpace(string(output)), ".")
	if len(splitVersion) < 3 {
		return "", fmt.Errorf("failed to parse dpkg version: %s", string(output))
	}
	return strings.Join(splitVersion[:3], "."), nil
}

func writeInstallInfo(tool, version string) error {
	info := installinfo.InstallMethod{
		Method: installinfo.InstallInfo{
			Tool:             tool,
			ToolVersion:      version,
			InstallerVersion: "updater_package",
		},
	}
	yamlData, err := yaml.Marshal(info)
	if err != nil {
		panic(err)
	}
	return os.WriteFile(installInfoFile, yamlData, 0644)
}

func writeInstallSignature() error {
	installSignature := map[string]string{
		"install_id":   strings.ToLower(uuid.New().String()),
		"install_type": "manual_update_via_apt",
		"install_time": strconv.FormatInt(time.Now().Unix(), 10),
	}
	jsonData, err := json.Marshal(installSignature)
	if err != nil {
		panic(err)
	}
	return os.WriteFile(installSigFile, jsonData, 0644)
}
