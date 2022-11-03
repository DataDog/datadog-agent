// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:generate go run github.com/tinylib/msgp -tests=false

package model

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// ProcessMonitoringEvent is an event sent by the ProcessMonitoring handler in the runtime-security module
type ProcessMonitoringEvent struct {
	*model.ProcessCacheEntry
	EventType      string    `json:"EventType" msg:"evt_type"`
	CollectionTime time.Time `json:"CollectionTime" msg:"collection_time"`
	ExitCode       uint32    `json:"ExitCode" msg:"exit_code"`
}

// ProcessEventToProcessMonitoringEvent converts a ProcessEvent to a ProcessMonitoringEvent
// It's used during tests to mock a ProcessMonitoringEvent message
// TODO(paulcacheux): remove this and associated tests/benchmarks
func ProcessEventToProcessMonitoringEvent(e *ProcessEvent) *ProcessMonitoringEvent {
	return &ProcessMonitoringEvent{
		EventType:      e.EventType.String(),
		CollectionTime: e.CollectionTime,
		ProcessCacheEntry: &model.ProcessCacheEntry{
			ProcessContext: model.ProcessContext{
				Process: model.Process{
					PIDContext: model.PIDContext{
						Pid: e.Pid,
					},
					ContainerID: e.ContainerID,
					PPid:        e.Ppid,
					Credentials: model.Credentials{
						UID:   e.UID,
						GID:   e.GID,
						User:  e.Username,
						Group: e.Group,
					},
					FileEvent: model.FileEvent{
						PathnameStr: e.Exe,
					},
					ArgsEntry: &model.ArgsEntry{
						Values: e.Cmdline,
					},
					ForkTime: e.ForkTime,
					ExecTime: e.ExecTime,
					ExitTime: e.ExitTime,
				},
			},
		},
		ExitCode: e.ExitCode,
	}
}
