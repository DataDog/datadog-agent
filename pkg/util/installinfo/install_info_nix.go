// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package installinfo offers helpers to interact with the 'install_info' file.
//
// The install_info files is present next to the agent configuration and contains information about how the agent was//
// installed and its version history.  The file is automatically updated by installation tools (MSI installer, Chef,
// Ansible, DPKG, ...).
package installinfo

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/google/uuid"
	"gopkg.in/yaml.v2"
)

const execTimeout = 30 * time.Second

var (
	configDir       = "/etc/datadog-agent"
	installInfoFile = filepath.Join(configDir, "install_info")
	installSigFile  = filepath.Join(configDir, "install.json")
)

// WriteInstallInfo write install info and signature files
func WriteInstallInfo(installerVersion, installType string) error {
	// avoid rewriting the files if they already exist
	if _, err := os.Stat(installInfoFile); err == nil {
		log.Info("Install info file already exists, skipping")
		return nil
	}
	tool, version := getToolVersion()
	if err := writeInstallInfo(tool, version, installerVersion); err != nil {
		return fmt.Errorf("failed to write install info file: %v", err)
	}
	if err := writeInstallSignature(installType); err != nil {
		return fmt.Errorf("failed to write install signature file: %v", err)
	}
	return nil
}

// RmInstallInfo removes the install info and signature files
func RmInstallInfo() {
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
		return tool, version
	}
	if _, err := exec.LookPath("rpm"); err == nil {
		tool = "rpm"
		toolVersion, err := getRPMVersion()
		if err == nil {
			version = fmt.Sprintf("rpm-%s", toolVersion)
		}
	}
	return tool, version
}

func getRPMVersion() (string, error) {
	cancelctx, cancelfunc := context.WithTimeout(context.Background(), execTimeout)
	defer cancelfunc()
	output, err := exec.CommandContext(cancelctx, "rpm", "-q", "-f", "/bin/rpm", "--queryformat", "%%{VERSION}").Output()
	return string(output), err
}

func getDpkgVersion() (string, error) {
	cancelctx, cancelfunc := context.WithTimeout(context.Background(), execTimeout)
	defer cancelfunc()
	cmd := exec.CommandContext(cancelctx, "dpkg-query", "--showformat=${Version}", "--show", "dpkg")
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

func writeInstallInfo(tool, version, installerVersion string) error {
	info := installInfoMethod{
		Method: InstallInfo{
			Tool:             tool,
			ToolVersion:      version,
			InstallerVersion: installerVersion,
		},
	}
	yamlData, err := yaml.Marshal(info)
	if err != nil {
		panic(err)
	}
	return os.WriteFile(installInfoFile, yamlData, 0644)
}

func writeInstallSignature(installType string) error {
	installSignature := map[string]string{
		"install_id":   strings.ToLower(uuid.New().String()),
		"install_type": installType,
		"install_time": strconv.FormatInt(time.Now().Unix(), 10),
	}
	jsonData, err := json.Marshal(installSignature)
	if err != nil {
		panic(err)
	}
	return os.WriteFile(installSigFile, jsonData, 0644)
}
