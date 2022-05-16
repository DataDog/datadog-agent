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

// ProcessMonitoringtoProcessEvent converts a ProcessMonitoringEvent to a generic ProcessEvent
func ProcessMonitoringtoProcessEvent(e *ProcessMonitoringEvent) *ProcessEvent {
	var cmdline []string
	if e.ArgsEntry != nil {
		cmdline = e.ArgsEntry.Values
	}

	return &ProcessEvent{
		EventType:      e.EventType,
		CollectionTime: e.CollectionTime,
		Pid:            e.Pid,
		Ppid:           e.PPid,
		UID:            e.UID,
		GID:            e.GID,
		Username:       e.User,
		Group:          e.Group,
		Exe:            e.FileEvent.PathnameStr,
		Cmdline:        cmdline,
		ForkTime:       e.ForkTime,
		ExecTime:       e.ExecTime,
		ExitTime:       e.ExitTime,
	}
}
