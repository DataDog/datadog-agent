// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package process

import (
	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	"github.com/DataDog/datadog-agent/pkg/process/events/model"
	smodel "github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// ProcessConsumer describes a process monitoring object
type ProcessConsumer struct{}

func (p *ProcessConsumer) Start() error {
	return nil
}

func (p *ProcessConsumer) Stop() {
}

// HandleEvent implement the EventHandler interface
func (p *ProcessConsumer) HandleEvent(event *smodel.Event) {
	// Force resolution of all event fields before exposing it through the API server
	event.ResolveFields()
	event.ResolveEventTimestamp()

	entry, _ := event.ResolveProcessCacheEntry()
	if entry == nil {
		return
	}

	var cmdline []string
	if entry.ArgsEntry != nil {
		// ignore if the args have been truncated
		cmdline = entry.ArgsEntry.Values
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

	_ = e

	// TODO

	/*
		data, err := e.MarshalMsg(nil)
		if err != nil {
			log.Error("Failed to marshal Process Lifecycle Event: ", err)
			return
		}


		p.module.SendProcessEvent(data)*/
}

// ID returns id for process monitor
func (p *ProcessConsumer) ID() string {
	return "PROCESS"
}

// NewProcessConsumer returns a new ProcessConsumer instance
func NewProcessConsumer(evm *eventmonitor.EventMonitor) (*ProcessConsumer, error) {
	p := &ProcessConsumer{}

	if err := evm.AddEventTypeHandler(smodel.ForkEventType, p); err != nil {
		return nil, err
	}
	if err := evm.AddEventTypeHandler(smodel.ExecEventType, p); err != nil {
		return nil, err
	}
	if err := evm.AddEventTypeHandler(smodel.ExitEventType, p); err != nil {
		return nil, err
	}

	return p, nil
}
