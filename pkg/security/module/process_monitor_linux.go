// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package module

import (
	"github.com/DataDog/datadog-agent/pkg/process/events/model"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// HandleEvent implement the EventHandler interface
func (p *ProcessMonitoring) HandleEvent(event *sprobe.Event) {
	// Force resolution of all event fields before exposing it through the API server
	event.ResolveFields(false)
	event.ResolveEventTimestamp()

	entry := event.ResolveProcessCacheEntry()
	if entry == nil {
		return
	}

	var cmdline []string
	if entry.ArgsEntry != nil {
		// ignore if the args have been truncated
		cmdline, _ = entry.ArgsEntry.ToArray()
	}

	e := &model.ProcessEvent{
		EventType:      model.NewEventType(event.GetEventType().String()),
		CollectionTime: event.Timestamp,
		Pid:            entry.Pid,
		ContainerID:    entry.ContainerID,
		Ppid:           entry.PPid,
		UID:            entry.UID,
		GID:            entry.GID,
		Username:       entry.User,
		Group:          entry.Group,
		Exe:            entry.FileEvent.PathnameStr, // FileEvent is not a pointer, so it can be directly accessed
		Cmdline:        cmdline,
		ForkTime:       entry.ForkTime,
		ExecTime:       entry.ExecTime,
		ExitTime:       entry.ExitTime,
		ExitCode:       event.Exit.Code,
	}

	data, err := e.MarshalMsg(nil)
	if err != nil {
		log.Error("Failed to marshal Process Lifecycle Event: ", err)
		return
	}

	p.module.apiServer.SendProcessEvent(data)
}