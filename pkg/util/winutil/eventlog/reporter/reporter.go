// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.
//go:build windows
// +build windows

package evtreporter

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
)

type Reporter interface {
	ReportEvent() evtapi.EventBookmarkHandle
	Close()
}

type reporter struct {
	eventLogAPI evtapi.API
	logHandle   evtapi.EventSourceHandle
}

func New(channelName string, api evtapi.API) (*reporter, error) {
	var r reporter

	if api == nil {
		return nil, fmt.Errorf("event log API is required")
	}
	r.eventLogAPI = api

	logHandle, err := r.eventLogAPI.RegisterEventSource(channelName)
	if err != nil {
		return nil, fmt.Errorf("Failed to register source %s: %v", channelName, err)
	}
	r.logHandle = logHandle

	return &r, nil
}

func (r *reporter) ReportEvent(
	Type uint,
	Category uint,
	EventID uint,
	Strings []string,
	RawData []uint8) error {
	return r.eventLogAPI.ReportEvent(
		r.logHandle,
		Type,
		Category,
		EventID,
		Strings,
		RawData)
}

func (r *reporter) Close() {
	if r.logHandle != evtapi.EventSourceHandle(0) {
		_ = r.eventLogAPI.DeregisterEventSource(r.logHandle)
	}
}
