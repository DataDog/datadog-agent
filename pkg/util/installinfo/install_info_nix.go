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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/google/uuid"
	"go.yaml.in/yaml/v2"
)

var (
	configDir       = "/etc/datadog-agent"
	installInfoFile = filepath.Join(configDir, "install_info")
	installSigFile  = filepath.Join(configDir, "install.json")
)

// WriteInstallInfo write install info and signature files
func WriteInstallInfo(tool, toolVersion, installType string) error {
	// avoid rewriting the files if they already exist
	if _, err := os.Stat(installInfoFile); err == nil {
		return nil
	}
	if err := writeInstallInfo(tool, toolVersion, installType); err != nil {
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
