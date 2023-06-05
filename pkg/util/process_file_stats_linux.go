// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package util

import (
	"os"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/shirou/gopsutil/v3/process"
)

// GetProcessFileStats returns the number of file handles the Agent process has open
func GetProcessFileStats() (*ProcessFileStats, error) {
	stats := ProcessFileStats{}

	// Creating a new process.Process type based on Agent PID
	p, err := process.NewProcess(int32(os.Getpid()))
	if err != nil {
		log.Debugf("Failed to retrieve agent process: %s. Process ID: %v", err, int32(os.Getpid()))
		return &stats, err
	}

	// Retrieving type []RlimitStat from struct process.Process p
	rs, err := p.RlimitUsage(true)
	if err != nil {
		log.Debugf("Failed to retrieve type RlimitStat: %s", err)
		return &stats, err
	}

	// Retrieving how many file handles the Core Agent process has open and the file limit set on the OS level
	stats.AgentOpenFiles = rs[process.RLIMIT_NOFILE].Used
	stats.OsFileLimit = rs[process.RLIMIT_NOFILE].Soft

	return &stats, nil
}
