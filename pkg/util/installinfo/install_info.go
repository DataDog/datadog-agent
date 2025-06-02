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
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
	"github.com/DataDog/datadog-agent/pkg/version"
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
func GetFilePath(conf model.Reader) string {
	return filepath.Join(configUtils.ConfFileDirectory(conf), "install_info")
}

// Get returns information about how the Agent was installed.
func Get(conf model.Reader) (*InstallInfo, error) {
	if installInfo, ok := getFromEnvVars(); ok {
		return installInfo, nil
	}
	return getFromPath(GetFilePath(conf))
}

func getFromEnvVars() (*InstallInfo, bool) {
	tool, okTool := os.LookupEnv("DD_INSTALL_INFO_TOOL")
	toolVersion, okToolVersion := os.LookupEnv("DD_INSTALL_INFO_TOOL_VERSION")
	installerVersion, okInstallerVersion := os.LookupEnv("DD_INSTALL_INFO_INSTALLER_VERSION")

	if !okTool || !okToolVersion || !okInstallerVersion {
		if okTool || okToolVersion || okInstallerVersion {
			log.Warnf("install info partially set through environment, ignoring: tool %t, version %t, installer %t", okTool, okToolVersion, okInstallerVersion)
		}
		return nil, false
	}

	return scrubFields(&InstallInfo{Tool: tool, ToolVersion: toolVersion, InstallerVersion: installerVersion}), true
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

	return scrubFields(&install.Method), nil
}

func scrubFields(info *InstallInfo) *InstallInfo {
	// Errors from ScrubString are only produced by the Reader interface, but
	// all these calls pass a string, which guarantees the Reader won't error
	info.Tool, _ = scrubber.ScrubString(info.Tool)
	info.ToolVersion, _ = scrubber.ScrubString(info.ToolVersion)
	info.InstallerVersion, _ = scrubber.ScrubString(info.InstallerVersion)
	return info
}

// LogVersionHistory loads version history file, append new entry if agent version is different than the last entry in the
// JSON file, trim the file if too many entries then save the file.
func LogVersionHistory() {
	versionHistoryFilePath := filepath.Join(pkgconfigsetup.Datadog().GetString("run_path"), "version-history.json")
	installInfoFilePath := GetFilePath(pkgconfigsetup.Datadog())
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
	if installInfo, ok := getFromEnvVars(); ok {
		newEntry.InstallMethod = *installInfo
	} else if installInfo, err := getFromPath(installInfoFilePath); err == nil {
		newEntry.InstallMethod = *installInfo
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
