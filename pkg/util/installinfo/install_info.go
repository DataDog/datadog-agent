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
	"net/http"
	"os"
	"path/filepath"
	"sync"
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

// Runtime override for install info - protected by mutex for concurrent access
var (
	runtimeInstallInfo *InstallInfo
	runtimeInfoMutex   sync.RWMutex
)

// SetInstallInfoRequest represents the JSON payload for setting install info
type SetInstallInfoRequest struct {
	Tool             string `json:"tool"`
	ToolVersion      string `json:"tool_version"`
	InstallerVersion string `json:"installer_version"`
}

// SetInstallInfoResponse represents the response after setting install info
type SetInstallInfoResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// GetFilePath returns the path of the 'install_info' directory relative to the loaded coinfiguration file. The
// 'install_info' directory contains information about how the agent was installed.
func GetFilePath(conf model.Reader) string {
	return filepath.Join(configUtils.ConfFileDirectory(conf), "install_info")
}

// Get returns information about how the Agent was installed.
func Get(conf model.Reader) (*InstallInfo, error) {
	if installInfo := getRuntimeInstallInfo(); installInfo != nil {
		return installInfo, nil
	}
	if installInfo, ok := getFromEnvVars(); ok {
		return installInfo, nil
	}
	return getFromPath(GetFilePath(conf))
}

// setRuntimeInstallInfo sets the install info at runtime, overriding file and env var values
func setRuntimeInstallInfo(info *InstallInfo) error {
	if info == nil {
		return fmt.Errorf("install info cannot be nil")
	}

	if info.Tool == "" || info.ToolVersion == "" || info.InstallerVersion == "" {
		return fmt.Errorf("install info must have tool, tool_version, and installer_version set")
	}

	runtimeInfoMutex.Lock()
	defer runtimeInfoMutex.Unlock()

	// Note: Unlike file/env-based sources which scrub on read, this scrubs the data at write time.
	// This means the original (unscrubbed) values are not retained.
	runtimeInstallInfo = scrubFields(&InstallInfo{
		Tool:             info.Tool,
		ToolVersion:      info.ToolVersion,
		InstallerVersion: info.InstallerVersion,
	})

	log.Infof("Runtime install info set: tool=%s, tool_version=%s, installer_version=%s",
		runtimeInstallInfo.Tool, runtimeInstallInfo.ToolVersion, runtimeInstallInfo.InstallerVersion)

	return nil
}

// getRuntimeInstallInfo returns the current runtime install info if set
func getRuntimeInstallInfo() *InstallInfo {
	runtimeInfoMutex.RLock()
	defer runtimeInfoMutex.RUnlock()

	if runtimeInstallInfo == nil {
		return nil
	}

	return &InstallInfo{
		Tool:             runtimeInstallInfo.Tool,
		ToolVersion:      runtimeInstallInfo.ToolVersion,
		InstallerVersion: runtimeInstallInfo.InstallerVersion,
	}
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
	if installInfo := getRuntimeInstallInfo(); installInfo != nil {
		newEntry.InstallMethod = *installInfo
	} else if installInfo, ok := getFromEnvVars(); ok {
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

// HandleSetInstallInfo is an HTTP handler for setting install info at runtime
func HandleSetInstallInfo(w http.ResponseWriter, r *http.Request) {
	var req SetInstallInfoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid JSON payload: "+err.Error())
		return
	}

	installInfo := &InstallInfo{
		Tool:             req.Tool,
		ToolVersion:      req.ToolVersion,
		InstallerVersion: req.InstallerVersion,
	}

	if err := setRuntimeInstallInfo(installInfo); err != nil {
		respondWithError(w, http.StatusBadRequest, "Failed to set install info: "+err.Error())
		return
	}

	respondWithSuccess(w, "Install info set successfully")
}

// HandleGetInstallInfo is an HTTP handler for getting current install info
func HandleGetInstallInfo(w http.ResponseWriter, _ *http.Request) {
	installInfo, err := Get(pkgconfigsetup.Datadog())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to get install info: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(installInfo); err != nil {
		log.Errorf("Failed to encode install info response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// Helper functions for HTTP responses
func respondWithError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := SetInstallInfoResponse{
		Success: false,
		Message: message,
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Errorf("Failed to encode error response: %v", err)
	}
}

func respondWithSuccess(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")

	response := SetInstallInfoResponse{
		Success: true,
		Message: message,
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Errorf("Failed to encode success response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}
