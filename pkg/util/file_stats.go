// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"encoding/json"
	"os"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/shirou/gopsutil/v3/process"
)

type FileStats struct {
	AgentOpenFiles float64 `json:"agent_open_files"`
	OsFileLimit    float64 `json:"os_file_limit"`
}

// getFileStats returns the number of files the Agent process has open
func GetFileStats() *FileStats {
	stats := FileStats{}

	// Creating a new process.Process type based on Agent PID
	p, err := process.NewProcess(int32(os.Getpid()))
	if err != nil {
		log.Errorf("Failed to retrieve agent process: %s", err)
		return &stats
	}

	// Retrieving []OpenFilesStat from Agent process.Process type
	files, err := p.OpenFiles()
	if err != nil {
		log.Errorf("Failed to retrieve agent process' open files slice: %s", err)
		return &stats
	}

	// Retrieving number of open files by getting the length of the Agent process.Process type's []OpenFilesStat slice
	stats.AgentOpenFiles = float64(len(files))

	// Retrieving type []RlimitStat from type process.Process p
	rs, err := p.Rlimit()
	if err != nil {
		log.Errorf("Failed to retrieve type RlimitStat: %s", err)
		return &stats
	}

	// Retrieving RLIMIT_NOFILE (index 7) from Agent process' RLimit[]
	openFilesMaxJSON := []byte(rs[7].String())
	openFilesMax := make(map[string]interface{})
	json.Unmarshal(openFilesMaxJSON, &openFilesMax)
	stats.OsFileLimit = openFilesMax["soft"].(float64)
	return &stats
}

func CheckFileStats(stats *FileStats) {
	// Log a warning if the ratio between the Agent's open files to the OS file limit is > 0.9, log an error if OS file limit is reached
	if stats.AgentOpenFiles/stats.OsFileLimit > 0.9 {
		log.Warnf("Agent process is close to OS file limit of %v. Agent process currently has %v files open.", stats.OsFileLimit, stats.AgentOpenFiles)
	} else if stats.AgentOpenFiles/stats.OsFileLimit >= 1 {
		log.Errorf("Agent process is reaching OS open file limit: %v. This may be preventing log files from being tailed by the Agent. Consider increasing OS file limit.", stats.OsFileLimit)
	}
	log.Debugf("Agent process currently has %v files open. OS file limit is currently set to %v.", stats.AgentOpenFiles, stats.OsFileLimit)
}
