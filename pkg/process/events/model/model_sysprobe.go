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
}

// ProcessMonitoringToProcessEvent converts a ProcessMonitoringEvent to a generic ProcessEvent
func ProcessMonitoringToProcessEvent(e *ProcessMonitoringEvent) *ProcessEvent {
	return &ProcessEvent{
		EventType:      e.EventType,
		CollectionTime: e.CollectionTime,
		Pid:            e.Pid,
		Ppid:           e.PPid,
		UID:            e.UID,
		GID:            e.GID,
		Username:       e.User,
		Group:          e.Group,
		Exe:            e.FileEvent.PathnameStr, // FileEvent is not a pointer, so it can be directly accessed
		Cmdline:        e.ScrubbedArgv,
		ForkTime:       e.ForkTime,
		ExecTime:       e.ExecTime,
		ExitTime:       e.ExitTime,
	}
}

// ProcessEventToProcessMonitoringEvent converts a ProcessEvent to a ProcessMonitoringEvent
// It's used during tests to mock a ProcessMonitoringEvent message
func ProcessEventToProcessMonitoringEvent(e *ProcessEvent) *ProcessMonitoringEvent {
	return &ProcessMonitoringEvent{
		EventType:      e.EventType,
		CollectionTime: e.CollectionTime,
		ProcessCacheEntry: &model.ProcessCacheEntry{
			ProcessContext: model.ProcessContext{
				Process: model.Process{
					PIDContext: model.PIDContext{
						Pid: e.Pid,
					},
					PPid: e.Ppid,
					Credentials: model.Credentials{
						UID:   e.UID,
						GID:   e.GID,
						User:  e.Username,
						Group: e.Group,
					},
					FileEvent: model.FileEvent{
						PathnameStr: e.Exe,
					},
					ScrubbedArgv: e.Cmdline,
					ForkTime:     e.ForkTime,
					ExecTime:     e.ExecTime,
					ExitTime:     e.ExitTime,
				},
			},
		},
	}
}

// NewMockedProcessMonitoringEvent returns a new mocked ProcessMonitoringEvent
func NewMockedProcessMonitoringEvent(evtType string, ts time.Time, pid uint32, exe string, args []string) *ProcessMonitoringEvent {
	var forkTime, execTime, exitTime time.Time
	switch evtType {
	case Fork:
		forkTime = ts
	case Exec:
		execTime = ts
	case Exit:
		exitTime = ts
	}

	return &ProcessMonitoringEvent{
		EventType:      evtType,
		CollectionTime: time.Now(),
		ProcessCacheEntry: &model.ProcessCacheEntry{
			ProcessContext: model.ProcessContext{
				Process: model.Process{
					PIDContext: model.PIDContext{
						Pid: pid,
					},
					PPid: 1,
					Credentials: model.Credentials{
						UID:   100,
						GID:   100,
						User:  "dog",
						Group: "dd-agent",
					},
					FileEvent: model.FileEvent{
						PathnameStr: exe,
					},
					ScrubbedArgv: args,
					ForkTime:     forkTime,
					ExecTime:     execTime,
					ExitTime:     exitTime,
				},
			},
		},
	}
}
