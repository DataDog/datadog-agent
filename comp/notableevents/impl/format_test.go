// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package notableeventsimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatAppCrashPayload(t *testing.T) {
	tests := []struct {
		name          string
		eventXML      string
		defaultTitle  string
		expectedTitle string
	}{
		{
			name: "extracts app name from named fields (Server 2025)",
			eventXML: `<Event xmlns="http://schemas.microsoft.com/win/2004/08/events/event">
				<System>
					<Provider Name="Application Error"/>
					<EventID>1000</EventID>
				</System>
				<EventData>
					<Data Name="AppName">notepad.exe</Data>
					<Data Name="AppVersion">10.0.19041.1</Data>
				</EventData>
			</Event>`,
			defaultTitle:  "Application crash",
			expectedTitle: "Application crash: notepad.exe",
		},
		{
			name: "extracts app name from positional list (Server 2022)",
			eventXML: `<Event xmlns="http://schemas.microsoft.com/win/2004/08/events/event">
				<System>
					<Provider Name="Application Error"/>
					<EventID>1000</EventID>
				</System>
				<EventData>
					<Data>notepad.exe</Data>
					<Data>10.0.20348.2227</Data>
				</EventData>
			</Event>`,
			defaultTitle:  "Application crash",
			expectedTitle: "Application crash: notepad.exe",
		},
		{
			name: "missing AppName keeps default title",
			eventXML: `<Event xmlns="http://schemas.microsoft.com/win/2004/08/events/event">
				<System>
					<Provider Name="Application Error"/>
					<EventID>1000</EventID>
				</System>
				<EventData>
					<Data Name="AppVersion">10.0.19041.1</Data>
				</EventData>
			</Event>`,
			defaultTitle:  "Application crash",
			expectedTitle: "Application crash",
		},
		{
			name: "empty EventData keeps default title",
			eventXML: `<Event xmlns="http://schemas.microsoft.com/win/2004/08/events/event">
				<System>
					<Provider Name="Application Error"/>
					<EventID>1000</EventID>
				</System>
				<EventData></EventData>
			</Event>`,
			defaultTitle:  "Application crash",
			expectedTitle: "Application crash",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eventMap, err := parseEventXML([]byte(tt.eventXML))
			require.NoError(t, err)

			payload := &eventPayload{Title: tt.defaultTitle}
			formatAppCrashPayload(payload, eventMap.Map)
			assert.Equal(t, tt.expectedTitle, payload.Title)
		})
	}
}

func TestFormatAppHangPayload(t *testing.T) {
	tests := []struct {
		name          string
		eventXML      string
		defaultTitle  string
		expectedTitle string
	}{
		{
			name: "extracts app name from named fields (Server 2025)",
			eventXML: `<Event xmlns="http://schemas.microsoft.com/win/2004/08/events/event">
				<System>
					<Provider Name="Application Hang"/>
					<EventID>1002</EventID>
				</System>
				<EventData>
					<Data Name="AppName">explorer.exe</Data>
					<Data Name="AppVersion">10.0.26100.7019</Data>
				</EventData>
			</Event>`,
			defaultTitle:  "Application hang",
			expectedTitle: "Application hang: explorer.exe",
		},
		{
			name: "extracts app name from positional list (Server 2022)",
			eventXML: `<Event xmlns="http://schemas.microsoft.com/win/2004/08/events/event">
				<System>
					<Provider Name="Application Hang"/>
					<EventID>1002</EventID>
				</System>
				<EventData>
					<Data>explorer.exe</Data>
					<Data>10.0.20348.1</Data>
				</EventData>
			</Event>`,
			defaultTitle:  "Application hang",
			expectedTitle: "Application hang: explorer.exe",
		},
		{
			name: "missing AppName keeps default title",
			eventXML: `<Event xmlns="http://schemas.microsoft.com/win/2004/08/events/event">
				<System>
					<Provider Name="Application Hang"/>
					<EventID>1002</EventID>
				</System>
				<EventData></EventData>
			</Event>`,
			defaultTitle:  "Application hang",
			expectedTitle: "Application hang",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eventMap, err := parseEventXML([]byte(tt.eventXML))
			require.NoError(t, err)

			payload := &eventPayload{Title: tt.defaultTitle}
			formatAppHangPayload(payload, eventMap.Map)
			assert.Equal(t, tt.expectedTitle, payload.Title)
		})
	}
}

func TestFormatWindowsUpdateFailedPayload(t *testing.T) {
	tests := []struct {
		name            string
		eventXML        string
		defaultTitle    string
		defaultMessage  string
		expectedTitle   string
		expectedMessage string
	}{
		{
			name: "extracts update title and error code",
			eventXML: `<Event xmlns="http://schemas.microsoft.com/win/2004/08/events/event">
				<System>
					<Provider Name="Microsoft-Windows-WindowsUpdateClient"/>
					<EventID>20</EventID>
				</System>
				<EventData>
					<Data Name="errorCode">0x80073d02</Data>
					<Data Name="updateTitle">9NTXGKQ8P7N0-MicrosoftWindows.CrossDevice</Data>
					<Data Name="updateGuid">{26dcdf7b-51f5-4f3b-9fe6-49e2520f37a6}</Data>
					<Data Name="updateRevisionNumber">1</Data>
					<Data Name="serviceGuid">{855e8a7c-ecb4-4ca3-b045-1dfa50104289}</Data>
				</EventData>
			</Event>`,
			defaultTitle:    "Failed Windows update",
			defaultMessage:  "A Windows Update installation failed",
			expectedTitle:   "Failed Windows update: 9NTXGKQ8P7N0-MicrosoftWindows.CrossDevice",
			expectedMessage: "Installation of 9NTXGKQ8P7N0-MicrosoftWindows.CrossDevice failed with error 0x80073d02",
		},
		{
			name: "only error code available",
			eventXML: `<Event xmlns="http://schemas.microsoft.com/win/2004/08/events/event">
				<System>
					<Provider Name="Microsoft-Windows-WindowsUpdateClient"/>
					<EventID>20</EventID>
				</System>
				<EventData>
					<Data Name="errorCode">0x80073d02</Data>
				</EventData>
			</Event>`,
			defaultTitle:    "Failed Windows update",
			defaultMessage:  "A Windows Update installation failed",
			expectedTitle:   "Failed Windows update",
			expectedMessage: "Windows Update failed with error 0x80073d02",
		},
		{
			name: "empty EventData keeps defaults",
			eventXML: `<Event xmlns="http://schemas.microsoft.com/win/2004/08/events/event">
				<System>
					<Provider Name="Microsoft-Windows-WindowsUpdateClient"/>
					<EventID>20</EventID>
				</System>
				<EventData></EventData>
			</Event>`,
			defaultTitle:    "Failed Windows update",
			defaultMessage:  "A Windows Update installation failed",
			expectedTitle:   "Failed Windows update",
			expectedMessage: "A Windows Update installation failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eventMap, err := parseEventXML([]byte(tt.eventXML))
			require.NoError(t, err)

			payload := &eventPayload{Title: tt.defaultTitle, Message: tt.defaultMessage}
			formatWindowsUpdateFailedPayload(payload, eventMap.Map)
			assert.Equal(t, tt.expectedTitle, payload.Title)
			assert.Equal(t, tt.expectedMessage, payload.Message)
		})
	}
}

func TestFormatMsiInstaller1033Payload(t *testing.T) {
	// Note: Successful installations (exit code 0) are filtered at the XPath query level,
	// so we only test failure cases here.
	tests := []struct {
		name            string
		eventXML        string
		defaultTitle    string
		defaultMessage  string
		expectedTitle   string
		expectedMessage string
	}{
		{
			name: "failed installation with exit code 1603",
			eventXML: `<Event xmlns="http://schemas.microsoft.com/win/2004/08/events/event">
				<System>
					<Provider Name="MsiInstaller"/>
					<EventID>1033</EventID>
				</System>
				<EventData>
					<Data>Datadog Agent</Data>
					<Data>7.75.0.0</Data>
					<Data>1033</Data>
					<Data>1603</Data>
					<Data>Datadog, Inc.</Data>
					<Data>(NULL)</Data>
				</EventData>
			</Event>`,
			defaultTitle:    "Failed application installation",
			defaultMessage:  "An application installation (MSI) failed",
			expectedTitle:   "Failed application installation: Datadog Agent 7.75.0.0",
			expectedMessage: "Installation of Datadog Agent failed with exit code 1603 (Fatal error during installation.)",
		},
		{
			name: "empty EventData keeps defaults",
			eventXML: `<Event xmlns="http://schemas.microsoft.com/win/2004/08/events/event">
				<System>
					<Provider Name="MsiInstaller"/>
					<EventID>1033</EventID>
				</System>
				<EventData></EventData>
			</Event>`,
			defaultTitle:    "Failed application installation",
			defaultMessage:  "An application installation (MSI) failed",
			expectedTitle:   "Failed application installation",
			expectedMessage: "An application installation (MSI) failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eventMap, err := parseEventXML([]byte(tt.eventXML))
			require.NoError(t, err)

			payload := &eventPayload{Title: tt.defaultTitle, Message: tt.defaultMessage}
			formatMsiInstaller1033Payload(payload, eventMap.Map)
			assert.Equal(t, tt.expectedTitle, payload.Title)
			assert.Equal(t, tt.expectedMessage, payload.Message)
		})
	}
}

func TestFormatMsiInstaller1034Payload(t *testing.T) {
	// Note: Successful removals (exit code 0) are filtered at the XPath query level,
	// so we only test failure cases here.
	tests := []struct {
		name            string
		eventXML        string
		defaultTitle    string
		defaultMessage  string
		expectedTitle   string
		expectedMessage string
	}{
		{
			name: "failed removal with exit code 1603",
			eventXML: `<Event xmlns="http://schemas.microsoft.com/win/2004/08/events/event">
				<System>
					<Provider Name="MsiInstaller"/>
					<EventID>1034</EventID>
				</System>
				<EventData>
					<Data>Datadog Agent</Data>
					<Data>7.74.0.0</Data>
					<Data>1033</Data>
					<Data>1603</Data>
					<Data>Datadog, Inc.</Data>
					<Data>(NULL)</Data>
				</EventData>
			</Event>`,
			defaultTitle:    "Failed application removal",
			defaultMessage:  "An application removal (MSI) failed",
			expectedTitle:   "Failed application removal: Datadog Agent 7.74.0.0",
			expectedMessage: "Removal of Datadog Agent failed with exit code 1603 (Fatal error during installation.)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eventMap, err := parseEventXML([]byte(tt.eventXML))
			require.NoError(t, err)

			payload := &eventPayload{Title: tt.defaultTitle, Message: tt.defaultMessage}
			formatMsiInstaller1034Payload(payload, eventMap.Map)
			assert.Equal(t, tt.expectedTitle, payload.Title)
			assert.Equal(t, tt.expectedMessage, payload.Message)
		})
	}
}

func TestFormatUnexpectedRebootPayload(t *testing.T) {
	tests := []struct {
		name              string
		eventXML          string
		defaultEventType  string
		defaultTitle      string
		defaultMessage    string
		expectedEventType string
		expectedTitle     string
		expectedMessage   string
	}{
		{
			name: "regular unexpected reboot (BugcheckCode=0) keeps defaults",
			eventXML: `<Event xmlns="http://schemas.microsoft.com/win/2004/08/events/event">
				<System>
					<Provider Name="Microsoft-Windows-Kernel-Power"/>
					<EventID>41</EventID>
				</System>
				<EventData>
					<Data Name="BugcheckCode">0</Data>
					<Data Name="BugcheckParameter1">0x0</Data>
					<Data Name="BugcheckParameter2">0x0</Data>
					<Data Name="BugcheckParameter3">0x0</Data>
					<Data Name="BugcheckParameter4">0x0</Data>
				</EventData>
			</Event>`,
			defaultEventType:  "Unexpected reboot",
			defaultTitle:      "Unexpected reboot",
			defaultMessage:    "The system has rebooted without cleanly shutting down first",
			expectedEventType: "Unexpected reboot",
			expectedTitle:     "Unexpected reboot",
			expectedMessage:   "The system has rebooted without cleanly shutting down first",
		},
		{
			name: "bugcheck-caused reboot (BugcheckCode=59) changes to Bugcheck event type",
			eventXML: `<Event xmlns="http://schemas.microsoft.com/win/2004/08/events/event">
				<System>
					<Provider Name="Microsoft-Windows-Kernel-Power"/>
					<EventID>41</EventID>
				</System>
				<EventData>
					<Data Name="BugcheckCode">59</Data>
					<Data Name="BugcheckParameter1">0xc0000005</Data>
					<Data Name="BugcheckParameter2">0xfffff8077a86e808</Data>
					<Data Name="BugcheckParameter3">0xfffff609cfac5f00</Data>
					<Data Name="BugcheckParameter4">0x0</Data>
				</EventData>
			</Event>`,
			defaultEventType:  "Unexpected reboot",
			defaultTitle:      "Unexpected reboot",
			defaultMessage:    "The system has rebooted without cleanly shutting down first",
			expectedEventType: "System crash",
			expectedTitle:     "System crash (bugcheck code:0x3B)",
			expectedMessage:   "The system crashed with bugcheck code 0x3B",
		},
		{
			name: "empty EventData keeps defaults",
			eventXML: `<Event xmlns="http://schemas.microsoft.com/win/2004/08/events/event">
				<System>
					<Provider Name="Microsoft-Windows-Kernel-Power"/>
					<EventID>41</EventID>
				</System>
				<EventData></EventData>
			</Event>`,
			defaultEventType:  "Unexpected reboot",
			defaultTitle:      "Unexpected reboot",
			defaultMessage:    "The system has rebooted without cleanly shutting down first",
			expectedEventType: "Unexpected reboot",
			expectedTitle:     "Unexpected reboot",
			expectedMessage:   "The system has rebooted without cleanly shutting down first",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eventMap, err := parseEventXML([]byte(tt.eventXML))
			require.NoError(t, err)

			payload := &eventPayload{
				EventType: tt.defaultEventType,
				Title:     tt.defaultTitle,
				Message:   tt.defaultMessage,
			}
			formatUnexpectedRebootPayload(payload, eventMap.Map)
			assert.Equal(t, tt.expectedEventType, payload.EventType)
			assert.Equal(t, tt.expectedTitle, payload.Title)
			assert.Equal(t, tt.expectedMessage, payload.Message)
		})
	}
}

// TestFormatMsiExitCode is a sanity check to ensure we can get the human readable error from the MSI exit codes.
func TestFormatMsiExitCode(t *testing.T) {
	tests := []struct {
		name     string
		exitCode string
		contains string // Substring we expect in the result
	}{
		{
			name:     "error 1603 - fatal error",
			exitCode: "1603",
			contains: "Fatal error during installation",
		},
		{
			name:     "error 1602 - user cancelled",
			exitCode: "1602",
			contains: "cancelled",
		},
		{
			name:     "error 1618 - already running",
			exitCode: "1618",
			contains: "Another installation is already in progress",
		},
		{
			name:     "invalid exit code returns original",
			exitCode: "not-a-number",
			contains: "not-a-number",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatMsiExitCode(tt.exitCode)
			assert.Contains(t, result, tt.contains)
		})
	}
}
