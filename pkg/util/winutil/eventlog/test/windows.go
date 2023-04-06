// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.
//go:build windows
// +build windows

package eventlog_test

import (
	"fmt"
	"testing"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api/windows"
)

const (
	eventLogRootKey = `SYSTEM\CurrentControlSet\Services\EventLog`
)

// WindowsAPITester uses the real Windows EventLog APIs
// and provides utilities to the test framework that will modify
// the host system (e.g. install event log source, generate events).
type WindowsAPITester struct {
	t           testing.TB
	eventlogapi *winevtapi.API
}

func NewWindowsAPITester(t testing.TB) *WindowsAPITester {
	var ti WindowsAPITester
	ti.t = t
	ti.eventlogapi = winevtapi.New()
	return &ti
}

func (ti *WindowsAPITester) Name() string {
	return "Windows"
}

func (ti *WindowsAPITester) T() testing.TB {
	return ti.t
}

func (ti *WindowsAPITester) API() evtapi.API {
	return ti.eventlogapi
}

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

	// eventcreate.exe is used because it has a generic message format that enabled EvtFormatMessage to succeed
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

func (ti *WindowsAPITester) GenerateEvents(channelName string, numEvents uint) error {

	sourceHandle, err := ti.eventlogapi.RegisterEventSource(channelName)
	if err != nil {
		return err
	}
	//nolint:errcheck
	defer ti.eventlogapi.DeregisterEventSource(sourceHandle)

	for i := uint(0); i < numEvents; i += 1 {
		err := ti.eventlogapi.ReportEvent(
			sourceHandle,
			windows.EVENTLOG_INFORMATION_TYPE,
			0, 1000, []string{"teststring"}, nil)
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
