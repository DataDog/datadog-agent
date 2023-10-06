// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package consumer

import (
	"github.com/DataDog/datadog-agent/pkg/process/events/model"
	smodel "github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// Copy copies the necessary fields from the event received from the event monitor
func (p *ProcessConsumer) Copy(event *smodel.Event) interface{} {
	// Force resolution of all event fields before exposing it through the API server
	event.ResolveFields()
	event.ResolveEventTime()

	entry := event.ProcessContext

	var cmdline []string
	if entry.ArgsEntry != nil {
		// ignore if the args have been truncated
		cmdline = entry.ArgsEntry.Values
	}

	return &model.ProcessEvent{
		EventType:      model.NewEventType(event.GetEventType().String()),
		CollectionTime: event.Timestamp,
		Pid:            entry.Pid,
		ContainerID:    entry.ContainerID,
		Ppid:           entry.PPid,
		Exe:            entry.FileEvent.PathnameStr, // FileEvent is not a pointer, so it can be directly accessed
		Cmdline:        cmdline,
		ExecTime:       entry.ExecTime,
		ExitTime:       entry.ExitTime,
		ExitCode:       event.Exit.Code,
	}
}
