// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package module

import (
	"github.com/DataDog/datadog-agent/pkg/process/events/model"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ProcessMonitoring describes a process monitoring object
type ProcessMonitoring struct {
	module *Module
}

// HandleEvent implement the EventHandler interface
func (p *ProcessMonitoring) HandleEvent(event *sprobe.Event) {
	// Force resolution of all event fields before exposing it through the API server
	event.ResolveFields()
	event.ResolveEventTimestamp()

	entry := event.ResolveProcessCacheEntry()
	if entry == nil {
		return
	}

	e := &model.ProcessMonitoringEvent{
		ProcessCacheEntry: entry,
		EventType:         event.GetEventType().String(),
		CollectionTime:    event.Timestamp,
		ExitCode:          event.Exit.Code,
	}

	data, err := e.MarshalMsg(nil)
	if err != nil {
		log.Error("Failed to marshal Process Lifecycle Event: ", err)
		return
	}

	p.module.apiServer.SendProcessEvent(data)
}

// HandleCustomEvent implement the EventHandler interface
func (p *ProcessMonitoring) HandleCustomEvent(rule *rules.Rule, event *sprobe.CustomEvent) {
}

// NewProcessMonitoring returns a new ProcessMonitoring instance
func NewProcessMonitoring(module *Module) *ProcessMonitoring {
	return &ProcessMonitoring{
		module: module,
	}
}
