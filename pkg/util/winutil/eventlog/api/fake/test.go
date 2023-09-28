// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.
//go:build windows
// +build windows

package fakeevtapi

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// helpers for FakeAPITester

func (api *API) AddEventLog(name string) error {
	// does it exist
	_, err := api.getEventLog(name)
	if err == nil {
		return fmt.Errorf("Event log %v already exists", name)
	}

	api.addEventLog(newEventLog(name))
	return nil
}

func (api *API) RemoveEventLog(name string) error {
	// Get event log
	_, err := api.getEventLog(name)
	if err != nil {
		return err
	}
	delete(api.eventLogs, name)
	return nil
}

func (api *API) AddEventSource(channel string, source string) error {
	// Get event log
	log, err := api.getEventLog(channel)
	if err != nil {
		return err
	}

	_, exists := log.sources[source]
	if !exists {
		var s eventSource
		s.name = source
		s.logName = channel
		log.sources[source] = &s
	}

	return nil
}

func (api *API) RemoveEventSource(channel string, name string) error {
	// Get event log
	log, err := api.getEventLog(channel)
	if err != nil {
		return err
	}
	delete(log.sources, name)
	return nil
}

func (api *API) GenerateEvents(sourceName string, numEvents uint) error {
	// find the log the source is registered to
	var eventLog *eventLog
	for _, log := range api.eventLogs {
		_, ok := log.sources[sourceName]
		if ok {
			eventLog = log
			break
		}
	}
	if eventLog == nil {
		return fmt.Errorf("Event source %v does not exist", sourceName)
	}

	// Use LocalSystem for the SID
	sid, _ := windows.CreateWellKnownSid(windows.WinLocalSystemSid)

	// Add junk events
	for i := uint(0); i < numEvents; i += 1 {
		_ = eventLog.reportEvent(api, windows.EVENTLOG_INFORMATION_TYPE,
			0, 1000, sid, []string{"teststring1", "teststring2"}, []uint8("AABBCCDD"))
	}

	return nil
}
