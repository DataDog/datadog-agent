// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.
//go:build windows

// Package evtreporter provides helpers for writing events to the Windows Event Log
package evtreporter

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// Reporter is an interface for writing an event to a Windows Event Log
type Reporter interface {
	ReportEvent(
		Type uint,
		Category uint,
		EventID uint,
		UserSID *windows.SID,
		Strings []string,
		RawData []uint8,
	) error
	Close()
}

type reporter struct {
	eventLogAPI evtapi.API
	logHandle   evtapi.EventSourceHandle
}

// New constructs a new Reporter.
// Call Close() when done to release resources.
func New(channelName string, api evtapi.API) (Reporter, error) {
	var r reporter

	if api == nil {
		return nil, fmt.Errorf("event log API is required")
	}
	r.eventLogAPI = api

	logHandle, err := r.eventLogAPI.RegisterEventSource(channelName)
	if err != nil {
		return nil, fmt.Errorf("Failed to register source %s: %w", channelName, err)
	}
	r.logHandle = logHandle

	return &r, nil
}

func (r *reporter) ReportEvent(
	Type uint,
	Category uint,
	EventID uint,
	UserSID *windows.SID,
	Strings []string,
	RawData []uint8) error {
	return r.eventLogAPI.ReportEvent(
		r.logHandle,
		Type,
		Category,
		EventID,
		UserSID,
		Strings,
		RawData)
}

func (r *reporter) Close() {
	if r.logHandle != evtapi.EventSourceHandle(0) {
		_ = r.eventLogAPI.DeregisterEventSource(r.logHandle)
	}
}
