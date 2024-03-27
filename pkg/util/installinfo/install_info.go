// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/config"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/google/uuid"
)

var (
	configDir       = "/etc/datadog-agent"
	installInfoFile = filepath.Join(configDir, "install_info")
	installSigFile  = filepath.Join(configDir, "install.json")
)

// InstallInfo contains metadata on how the Agent was installed
type InstallInfo struct {
	Tool             string `json:"tool" yaml:"tool"`
	ToolVersion      string `json:"tool_version" yaml:"tool_version"`
	InstallerVersion string `json:"installer_version" yaml:"installer_version"`
}

// installInfoMethod contains install info
type installInfoMethod struct {
	Method InstallInfo `json:"install_method" yaml:"install_method"`
}

type versionHistoryEntry struct {
	Version       string      `json:"version"`
	Timestamp     time.Time   `json:"timestamp"`
	InstallMethod InstallInfo `json:"install_method" yaml:"install_method"`
}

type versionHistoryEntries struct {
	Entries []versionHistoryEntry `json:"entries"`
}

const maxVersionHistoryEntries = 60

// GetFilePath returns the path of the 'install_info' directory relative to the loaded coinfiguration file. The
// 'install_info' directory contains information about how the agent was installed.
func GetFilePath(conf config.Reader) string {
	return filepath.Join(configUtils.ConfFileDirectory(conf), "install_info")
}

// Get returns information about how the Agent was installed.
func Get(conf config.Reader) (*InstallInfo, error) {
	return getFromPath(GetFilePath(conf))
}

func getFromPath(path string) (*InstallInfo, error) {
	yamlContent, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var install installInfoMethod
	if err := yaml.UnmarshalStrict(yamlContent, &install); err != nil {
		// file was manipulated and is not relevant to format
		return nil, err
	}

	return &install.Method, nil
}

// LogVersionHistory loads version history file, append new entry if agent version is different than the last entry in the
// JSON file, trim the file if too many entries then save the file.
func LogVersionHistory() {
	versionHistoryFilePath := filepath.Join(config.Datadog.GetString("run_path"), "version-history.json")
	installInfoFilePath := GetFilePath(config.Datadog)
	logVersionHistoryToFile(versionHistoryFilePath, installInfoFilePath, version.AgentVersion, time.Now().UTC())
}

func logVersionHistoryToFile(versionHistoryFilePath, installInfoFilePath, agentVersion string, timestamp time.Time) {
	if agentVersion == "" || timestamp.IsZero() {
		return
	}

	history := versionHistoryEntries{}
	file, err := os.ReadFile(versionHistoryFilePath)
	if err != nil {
		log.Infof("Cannot read file: %s, will create a new one. %v", versionHistoryFilePath, err)
	} else {
		err = json.Unmarshal(file, &history)
		if err != nil {
			// If file is in illegal format, ignore the error and regenerate the file.
			log.Errorf("Cannot deserialize json file: %s. %v", versionHistoryFilePath, err)
		}
	}

	// Only append the version info if no entry or this is different than the last entry.
	if len(history.Entries) != 0 && history.Entries[len(history.Entries)-1].Version == agentVersion {
		return
	}

	newEntry := versionHistoryEntry{
		Version:   agentVersion,
		Timestamp: timestamp,
	}
	info, err := getFromPath(installInfoFilePath)
	if err == nil {
		newEntry.InstallMethod = *info
	} else {
		log.Infof("Cannot read %s: %s", installInfoFilePath, err)
	}

	history.Entries = append(history.Entries, newEntry)

	// Trim entries if they grow beyond the max capacity.
	itemsToTrim := len(history.Entries) - maxVersionHistoryEntries
	if itemsToTrim > 0 {
		copy(history.Entries[0:], history.Entries[itemsToTrim:])
		history.Entries = history.Entries[:maxVersionHistoryEntries]
	}

	file, err = json.Marshal(history)
	if err != nil {
		log.Errorf("Cannot serialize json file: %s %v", versionHistoryFilePath, err)
		return
	}

	err = os.WriteFile(versionHistoryFilePath, file, 0644)
	if err != nil {
		log.Errorf("Cannot write json file: %s %v", versionHistoryFilePath, err)
		return
	}
}

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
