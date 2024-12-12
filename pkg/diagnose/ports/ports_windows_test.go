// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ports

import (
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestRetrieveProcessName(t *testing.T) {
	// Save the original function and restore it after the test
	originalNtQuerySystemInformation := ntQuerySystemInformation
	defer func() { ntQuerySystemInformation = originalNtQuerySystemInformation }()

	// Mock ntQuerySystemInformation to return a known process name "agent.exe"
	ntQuerySystemInformation = func(
		systemInformationClass int32,
		systemInformation unsafe.Pointer,
		systemInformationLength uint32,
		returnLength *uint32,
	) error {
		processInfo := (*SystemProcessIDInformation)(systemInformation)

		// We want to simulate that NtQuerySystemInformation returned "agent.exe"
		testName := "agent.exe"
		utf16Name, err := windows.UTF16PtrFromString(testName)
		if err != nil {
			return err
		}

		processInfo.ImageName.Buffer = utf16Name
		processInfo.ImageName.Length = uint16(len(testName) * 2) // length in bytes
		processInfo.ImageName.MaximumLength = uint16((len(testName) + 1) * 2)

		return nil
	}

	// Now call RetrieveProcessName with a dummy PID
	processName, err := RetrieveProcessName(1234, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "agent"
	if processName != expected {
		t.Errorf("expected %q, got %q", expected, processName)
	}
}
