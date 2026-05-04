// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package installinfoimpl implements the installinfo component.
package installinfoimpl

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/comp/agent/installinfo/def"
	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// installInfoMethod is the YAML envelope used in the install_info file.
type installInfoMethod struct {
	Method installinfo.InstallInfo `json:"install_method" yaml:"install_method"`
}

type versionHistoryEntry struct {
	Version       string                  `json:"version"`
	Timestamp     time.Time               `json:"timestamp"`
	InstallMethod installinfo.InstallInfo `json:"install_method" yaml:"install_method"`
}

type versionHistoryEntries struct {
	Entries []versionHistoryEntry `json:"entries"`
}

const maxVersionHistoryEntries = 60

// Requires defines the dependencies for the installinfo component.
type Requires struct {
	LC     compdef.Lifecycle
	Config config.Component
}

// Provides defines the output of the installinfo component.
type Provides struct {
	Comp          installinfo.Component
	FlareProvider flaretypes.Provider
	GetEndpoint   api.AgentEndpointProvider
	SetEndpoint   api.AgentEndpointProvider
}

type impl struct {
	conf config.Component

	mu          sync.RWMutex
	runtimeInfo *installinfo.InstallInfo
}

// NewComponent creates a new installinfo component.
func NewComponent(deps Requires) Provides {
	i := &impl{conf: deps.Config}
	deps.LC.Append(compdef.Hook{
		OnStart: func(context.Context) error {
			i.logVersionHistory()
			return nil
		},
	})
	return Provides{
		Comp:          i,
		FlareProvider: flaretypes.NewProvider(i.fillFlare),
		GetEndpoint:   api.NewAgentEndpointProvider(i.handleGet, "/install-info", "GET"),
		SetEndpoint:   api.NewAgentEndpointProvider(i.handleSet, "/install-info", "POST", "PUT"),
	}
}

// Get returns information about how the Agent was installed.
// The runtime override takes precedence over env vars and the install_info file.
func (i *impl) Get() (*installinfo.InstallInfo, error) {
	if info := i.getRuntimeInfo(); info != nil {
		return info, nil
	}
	return i.get()
}

func (i *impl) logVersionHistory() {
	versionHistoryFilePath := filepath.Join(i.conf.GetString("run_path"), "version-history.json")
	logVersionHistoryToFile(versionHistoryFilePath, i.getFilePath(), version.AgentVersion, time.Now().UTC(), i.getRuntimeInfo())
}

func (i *impl) getFilePath() string {
	return filepath.Join(configUtils.ConfFileDirectory(i.conf), "install_info")
}

// get reads install info from env vars then file, without the runtime override.
func (i *impl) get() (*installinfo.InstallInfo, error) {
	if info, ok := getFromEnvVars(); ok {
		return info, nil
	}
	return getFromPath(i.getFilePath())
}

// set stores a runtime override for install info.
func (i *impl) set(info *installinfo.InstallInfo) error {
	if info == nil {
		return errors.New("install info cannot be nil")
	}
	if info.Tool == "" || info.ToolVersion == "" || info.InstallerVersion == "" {
		return errors.New("install info must have tool, tool_version, and installer_version set")
	}
	scrubbed := scrubInfo(&installinfo.InstallInfo{
		Tool:             info.Tool,
		ToolVersion:      info.ToolVersion,
		InstallerVersion: info.InstallerVersion,
	})
	i.mu.Lock()
	defer i.mu.Unlock()
	i.runtimeInfo = scrubbed
	log.Infof("Runtime install info set: tool=%s, tool_version=%s, installer_version=%s",
		scrubbed.Tool, scrubbed.ToolVersion, scrubbed.InstallerVersion)
	return nil
}

func (i *impl) getRuntimeInfo() *installinfo.InstallInfo {
	i.mu.RLock()
	defer i.mu.RUnlock()
	if i.runtimeInfo == nil {
		return nil
	}
	return &installinfo.InstallInfo{
		Tool:             i.runtimeInfo.Tool,
		ToolVersion:      i.runtimeInfo.ToolVersion,
		InstallerVersion: i.runtimeInfo.InstallerVersion,
	}
}

func getFromEnvVars() (*installinfo.InstallInfo, bool) {
	tool, okTool := os.LookupEnv("DD_INSTALL_INFO_TOOL")
	toolVersion, okToolVersion := os.LookupEnv("DD_INSTALL_INFO_TOOL_VERSION")
	installerVersion, okInstallerVersion := os.LookupEnv("DD_INSTALL_INFO_INSTALLER_VERSION")

	if !okTool || !okToolVersion || !okInstallerVersion {
		if okTool || okToolVersion || okInstallerVersion {
			log.Warnf("install info partially set through environment, ignoring: tool %t, version %t, installer %t", okTool, okToolVersion, okInstallerVersion)
		}
		return nil, false
	}
	return scrubInfo(&installinfo.InstallInfo{Tool: tool, ToolVersion: toolVersion, InstallerVersion: installerVersion}), true
}

func getFromPath(path string) (*installinfo.InstallInfo, error) {
	yamlContent, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var install installInfoMethod
	if err := yaml.UnmarshalStrict(yamlContent, &install); err != nil {
		return nil, err
	}
	return scrubInfo(&install.Method), nil
}

func scrubInfo(info *installinfo.InstallInfo) *installinfo.InstallInfo {
	info.Tool, _ = scrubber.ScrubString(info.Tool)
	info.ToolVersion, _ = scrubber.ScrubString(info.ToolVersion)
	info.InstallerVersion, _ = scrubber.ScrubString(info.InstallerVersion)
	return info
}

func logVersionHistoryToFile(versionHistoryFilePath, installInfoFilePath, agentVersion string, timestamp time.Time, runtimeOverride *installinfo.InstallInfo) {
	if agentVersion == "" || timestamp.IsZero() {
		return
	}

	history := versionHistoryEntries{}
	file, err := os.ReadFile(versionHistoryFilePath)
	if err != nil {
		log.Infof("Cannot read file: %s, will create a new one. %v", versionHistoryFilePath, err)
	} else {
		if err = json.Unmarshal(file, &history); err != nil {
			log.Errorf("Cannot deserialize json file: %s. %v", versionHistoryFilePath, err)
		}
	}

	if len(history.Entries) != 0 && history.Entries[len(history.Entries)-1].Version == agentVersion {
		return
	}

	newEntry := versionHistoryEntry{Version: agentVersion, Timestamp: timestamp}
	if runtimeOverride != nil {
		newEntry.InstallMethod = *runtimeOverride
	} else if info, ok := getFromEnvVars(); ok {
		newEntry.InstallMethod = *info
	} else if info, err := getFromPath(installInfoFilePath); err == nil {
		newEntry.InstallMethod = *info
	} else {
		log.Infof("Cannot read %s: %s", installInfoFilePath, err)
	}

	history.Entries = append(history.Entries, newEntry)

	if itemsToTrim := len(history.Entries) - maxVersionHistoryEntries; itemsToTrim > 0 {
		copy(history.Entries[0:], history.Entries[itemsToTrim:])
		history.Entries = history.Entries[:maxVersionHistoryEntries]
	}

	file, err = json.Marshal(history)
	if err != nil {
		log.Errorf("Cannot serialize json file: %s %v", versionHistoryFilePath, err)
		return
	}
	if err = os.WriteFile(versionHistoryFilePath, file, 0644); err != nil {
		log.Errorf("Cannot write json file: %s %v", versionHistoryFilePath, err)
	}
}

func (i *impl) fillFlare(_ context.Context, fb flaretypes.FlareBuilder) error {
	fb.CopyFileTo(i.getFilePath(), "install_info.log") //nolint:errcheck
	return nil
}

type setInstallInfoRequest struct {
	Tool             string `json:"tool"`
	ToolVersion      string `json:"tool_version"`
	InstallerVersion string `json:"installer_version"`
}

type setInstallInfoResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func (i *impl) handleGet(w http.ResponseWriter, _ *http.Request) {
	info, err := i.Get()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to get install info: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(info); err != nil {
		log.Errorf("Failed to encode install info response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (i *impl) handleSet(w http.ResponseWriter, r *http.Request) {
	var req setInstallInfoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid JSON payload: "+err.Error())
		return
	}
	if err := i.set(&installinfo.InstallInfo{
		Tool:             req.Tool,
		ToolVersion:      req.ToolVersion,
		InstallerVersion: req.InstallerVersion,
	}); err != nil {
		respondError(w, http.StatusBadRequest, "Failed to set install info: "+err.Error())
		return
	}
	respondSuccess(w, "Install info set successfully")
}

func respondError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(setInstallInfoResponse{Success: false, Message: message}) //nolint:errcheck
}

func respondSuccess(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(setInstallInfoResponse{Success: true, Message: message}); err != nil {
		log.Errorf("Failed to encode success response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}
