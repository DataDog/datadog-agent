// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

package fakeevtapi

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// helpers for FakeAPITester

// AddEventLog adds a new event log (channel) to the fake API
func (api *API) AddEventLog(name string) error {
	// does it exist
	_, err := api.getEventLog(name)
	if err == nil {
		return fmt.Errorf("Event log %v already exists", name)
	}

	api.addEventLog(newEventLog(name))
	return nil
}

// RemoveEventLog removes an event log (channel) from the fake API
func (api *API) RemoveEventLog(name string) error {
	// Get event log
	_, err := api.getEventLog(name)
	if err != nil {
		return err
	}
	delete(api.eventLogs, name)
	return nil
}

// AddEventSource adds a new source to an existing event log/channel
func (api *API) AddEventSource(channel string, source string) error {
	// Get event log
	log, err := api.getEventLog(channel)
	if err != nil {
		return err
	}
	log.mu.Lock()
	defer log.mu.Unlock()

	_, exists := log.sources[source]
	if !exists {
		var s eventSource
		s.name = source
		s.logName = channel
		log.sources[source] = &s
	}

	return nil
}

// RemoveEventSource removes a source from a event log/channel
func (api *API) RemoveEventSource(channel string, name string) error {
	// Get event log
	log, err := api.getEventLog(channel)
	if err != nil {
		return err
	}
	log.mu.Lock()
	defer log.mu.Unlock()
	delete(log.sources, name)
	return nil
}

// GenerateEvents writes @numEvents to the @sourceName event log source to help with
// generating events for testing.
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
	eventLog.mu.Lock()
	defer eventLog.mu.Unlock()

	// Use LocalSystem for the SID
	sid, _ := windows.CreateWellKnownSid(windows.WinLocalSystemSid)

	// Add junk events
	for i := uint(0); i < numEvents; i++ {
		_ = eventLog.reportEvent(api, windows.EVENTLOG_INFORMATION_TYPE,
			0, 1000, sid, []string{"teststring1", "teststring2"}, []uint8("AABBCCDD"))
	}

	return nil
}
