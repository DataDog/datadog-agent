// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.
//go:build windows

package eventlog_test

import (
	"fmt"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api/windows"
)

const (
	eventLogRootKey = `SYSTEM\CurrentControlSet\Services\EventLog`
	// WindowsAPIName identifies this API in GetAPITesterByName
	WindowsAPIName = "Windows"
)

// WindowsAPITester uses the real Windows EventLog APIs
// and provides utilities to the test framework that will modify
// the host system (e.g. install event log source, generate events).
type WindowsAPITester struct {
	eventlogapi *winevtapi.API
}

// NewWindowsAPITester constructs a new API tester for the Windows Event Log API
func NewWindowsAPITester() *WindowsAPITester {
	var ti WindowsAPITester
	ti.eventlogapi = winevtapi.New()
	return &ti
}

// Name returns the name that identifies this tester for use by GetAPITesterByName
func (ti *WindowsAPITester) Name() string {
	return WindowsAPIName
}

// API returns the Windows API
func (ti *WindowsAPITester) API() evtapi.API {
	return ti.eventlogapi
}

// InstallChannel adds a new event log.
// New event logs are created by creating registry keys.
func (ti *WindowsAPITester) InstallChannel(channel string) error {
	// Open EventLog registry key
	rootKey, err := registry.OpenKey(registry.LOCAL_MACHINE, channelRootKey(), registry.CREATE_SUB_KEY)
	if err != nil {
		return err
	}
	defer rootKey.Close()

	// Create the channel subkey
	channelKey, _, err := registry.CreateKey(rootKey, channel, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer channelKey.Close()

	// Increase the max size to accommodate more events
	// The default is 20MB, we increase it to 100MB
	err = channelKey.SetDWordValue("MaxSize", 100*1024*1024)
	if err != nil {
		return err
	}

	return nil
}

// InstallSource adds a new source to an event log.
// New sources are created by creating registry keys.
// eventcreate.exe is used for the EventMessageFile because it has a generic message
// format that enables EvtFormatMessage to succeed
func (ti *WindowsAPITester) InstallSource(channel string, source string) error {
	// Open channel key
	channelKey, err := registry.OpenKey(registry.LOCAL_MACHINE, channelRegistryKey(channel), registry.CREATE_SUB_KEY)
	if err != nil {
		return err
	}
	defer channelKey.Close()

	// Create the source subkey
	sourceKey, _, err := registry.CreateKey(channelKey, source, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer sourceKey.Close()

	err = sourceKey.SetExpandStringValue("EventMessageFile", `%SystemRoot%\System32\eventcreate.exe`)
	if err != nil {
		return err
	}
	err = sourceKey.SetDWordValue("TypesSupported", windows.EVENTLOG_INFORMATION_TYPE|windows.EVENTLOG_WARNING_TYPE|windows.EVENTLOG_ERROR_TYPE)
	if err != nil {
		return err
	}

	return nil
}

// RemoveChannel removes an event log.
// Event logs are removed by deleting registry keys.
func (ti *WindowsAPITester) RemoveChannel(channel string) error {
	// Open EventLog registry key
	rootKey, err := registry.OpenKey(registry.LOCAL_MACHINE, channelRootKey(), registry.CREATE_SUB_KEY)
	if err != nil {
		return err
	}
	defer rootKey.Close()

	// Delete channel subkey
	return registry.DeleteKey(rootKey, channel)
}

// RemoveSource removes a source from an event log channel.
// Sources are removed by deleting registry keys.
func (ti *WindowsAPITester) RemoveSource(channel string, source string) error {
	// Open channel key
	channelKey, err := registry.OpenKey(registry.LOCAL_MACHINE, channelRegistryKey(channel), registry.CREATE_SUB_KEY)
	if err != nil {
		return err
	}
	defer channelKey.Close()

	// Delete source subkey
	return registry.DeleteKey(channelKey, source)
}

// GenerateEvents creates numEvents new event records
func (ti *WindowsAPITester) GenerateEvents(channelName string, numEvents uint) error {

	sourceHandle, err := ti.eventlogapi.RegisterEventSource(channelName)
	if err != nil {
		return err
	}
	//nolint:errcheck
	defer ti.eventlogapi.DeregisterEventSource(sourceHandle)

	// Use LocalSystem for the SID
	sid, _ := windows.CreateWellKnownSid(windows.WinLocalSystemSid)

	for i := uint(0); i < numEvents; i++ {
		err := ti.eventlogapi.ReportEvent(
			sourceHandle,
			windows.EVENTLOG_INFORMATION_TYPE,
			0, 1000, sid, []string{"teststring"}, nil)
		if err != nil {
			return err
		}
	}

	return nil
}

func channelRootKey() string {
	return eventLogRootKey
}

func channelRegistryKey(channel string) string {
	return fmt.Sprintf(`%v\%v`, channelRootKey(), channel)
}
