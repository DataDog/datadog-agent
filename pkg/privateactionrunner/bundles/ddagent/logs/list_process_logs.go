// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_ddagent_logs

import (
	"context"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// ProcessLogEntry represents a single process with its associated log files.
type ProcessLogEntry struct {
	PID         int32    `json:"pid"`
	Name        string   `json:"name"`
	Exe         string   `json:"exe"`
	ServiceName string   `json:"serviceName"`
	LogFiles    []string `json:"logFiles"`
}

// ListProcessLogsOutputs is the output returned by the listProcessLogs action.
type ListProcessLogsOutputs struct {
	Processes []ProcessLogEntry `json:"processes"`
}

// ListProcessLogsHandler implements the listProcessLogs action.
type ListProcessLogsHandler struct {
	wmeta workloadmeta.Component
}

// NewListProcessLogsHandler creates a new ListProcessLogsHandler.
func NewListProcessLogsHandler(wmeta workloadmeta.Component) *ListProcessLogsHandler {
	return &ListProcessLogsHandler{wmeta: wmeta}
}

// Run executes the listProcessLogs action.
func (h *ListProcessLogsHandler) Run(
	_ context.Context,
	_ *types.Task,
	_ *privateconnection.PrivateCredentials,
) (interface{}, error) {
	processes := h.wmeta.ListProcesses()
	entries := make([]ProcessLogEntry, 0)
	for _, proc := range processes {
		if proc.Service == nil || len(proc.Service.LogFiles) == 0 {
			continue
		}
		entries = append(entries, ProcessLogEntry{
			PID:         proc.Pid,
			Name:        proc.Name,
			Exe:         proc.Exe,
			ServiceName: proc.Service.GeneratedName,
			LogFiles:    proc.Service.LogFiles,
		})
	}
	return &ListProcessLogsOutputs{Processes: entries}, nil
}
