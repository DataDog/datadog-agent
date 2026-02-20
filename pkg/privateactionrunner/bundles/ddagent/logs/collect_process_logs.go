// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_ddagent_logs

import (
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// collectProcessLogs queries workloadmeta for all processes and extracts their
// associated log files. Paths from workloadmeta are already host-relative.
func collectProcessLogs(wmeta workloadmeta.Component) []FileEntry {
	if wmeta == nil {
		return nil
	}

	processes := wmeta.ListProcesses()
	var entries []FileEntry
	for _, proc := range processes {
		if proc.Service == nil || len(proc.Service.LogFiles) == 0 {
			continue
		}
		for _, logPath := range proc.Service.LogFiles {
			entries = append(entries, FileEntry{
				Path:        logPath,
				Source:      "process",
				ProcessName: proc.Name,
				PID:         proc.Pid,
				ServiceName: proc.Service.GeneratedName,
			})
		}
	}
	return entries
}
