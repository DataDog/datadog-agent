// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
	"gopkg.in/yaml.v2"
)

type versionHistoryEntry struct {
	Version       string        `json:"version"`
	Timestamp     time.Time     `json:"timestamp"`
	InstallMethod installMethod `json:"install_method" yaml:"install_method"`
}

type installMethod struct {
	Tool             string `json:"tool" yaml:"tool"`
	ToolVersion      string `json:"tool_version" yaml:"tool_version"`
	InstallerVersion string `json:"installer_version" yaml:"installer_version"`
}

type versionHistoryEntries struct {
	Entries []versionHistoryEntry `json:"entries"`
}

const maxVersionHistoryEntries = 60

// LogVersionHistory loads version history file, append new entry if agent version is different than the last entry in the
// JSON file, trim the file if too many entries then save the file.
func LogVersionHistory() {
	versionHistoryFilePath := filepath.Join(config.Datadog.GetString("run_path"), "version-history.json")
	installInfoFilePath := filepath.Join(config.FileUsedDir(), "install_info")
	logVersionHistoryToFile(versionHistoryFilePath, installInfoFilePath, version.AgentVersion, time.Now().UTC())
}

func logVersionHistoryToFile(versionHistoryFilePath, installInfoFilePath, agentVersion string, timestamp time.Time) {
	file, err := ioutil.ReadFile(versionHistoryFilePath)

	history := versionHistoryEntries{}

	if err != nil {
		log.Infof("Cannot read file: %s, will create a new one. %v", versionHistoryFilePath, err)
	} else {
		err = json.Unmarshal(file, &history)
		if err != nil {
			// If file is in illegal format, ignore the error and regenerate the file.
			log.Errorf("Cannot deserialize json file: %s. %v", versionHistoryFilePath, err)
		}
	}

	newEntry := versionHistoryEntry{
		Version:   agentVersion,
		Timestamp: timestamp,
	}

	installInfo, err := ioutil.ReadFile(installInfoFilePath)
	if err != nil {
		log.Infof("Cannot read %s: %s", installInfoFilePath, err)
	}
	if err := yaml.Unmarshal(installInfo, &newEntry); err != nil {
		log.Infof("Cannot yaml unmarshal %s: %s", installInfoFilePath, err)
	}

	if len(history.Entries) == 0 || history.Entries[len(history.Entries)-1].Version != newEntry.Version {
		// Only append the version info if no entry or this is different than the last entry.
		history.Entries = append(history.Entries, newEntry)
	} else {
		// Otherwise no change is needed, just return.
		return
	}

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

	err = ioutil.WriteFile(versionHistoryFilePath, file, 0644)
	if err != nil {
		log.Errorf("Cannot write json file: %s %v", versionHistoryFilePath, err)
		return
	}
}
