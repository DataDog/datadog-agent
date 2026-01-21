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
			name: "extracts app name from event XML",
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
			name: "extracts app name from event XML",
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

func TestFormatMsiInstallerPayload(t *testing.T) {
	tests := []struct {
		name            string
		eventXML        string
		defaultTitle    string
		defaultMessage  string
		expectedTitle   string
		expectedMessage string
	}{
		{
			name: "parses product name and error from XML",
			eventXML: `<Event xmlns="http://schemas.microsoft.com/win/2004/08/events/event">
				<System>
					<Provider Name="MsiInstaller"/>
					<EventID>11708</EventID>
				</System>
				<EventData>
					<Data>Product: Datadog Agent -- Automatic downgrades are not supported. Uninstall the current version, and then reinstall the desired version.</Data>
					<Data>(NULL)</Data>
				</EventData>
			</Event>`,
			defaultTitle:    "Failed application installation",
			defaultMessage:  "An application installation (MSI) failed",
			expectedTitle:   "Failed application installation: Datadog Agent",
			expectedMessage: "Product: Datadog Agent -- Automatic downgrades are not supported. Uninstall the current version, and then reinstall the desired version.",
		},
		{
			name: "parses product with complex error message",
			eventXML: `<Event xmlns="http://schemas.microsoft.com/win/2004/08/events/event">
				<System>
					<Provider Name="MsiInstaller"/>
					<EventID>11708</EventID>
				</System>
				<EventData>
					<Data>Product: Slack (Machine - MSI) -- Error 1714. The older version of Slack (Machine - MSI) cannot be removed. Contact your technical support group. System Error 1612.</Data>
					<Data>(NULL)</Data>
				</EventData>
			</Event>`,
			defaultTitle:    "Failed application installation",
			defaultMessage:  "An application installation (MSI) failed",
			expectedTitle:   "Failed application installation: Slack (Machine - MSI)",
			expectedMessage: "Product: Slack (Machine - MSI) -- Error 1714. The older version of Slack (Machine - MSI) cannot be removed. Contact your technical support group. System Error 1612.",
		},
		{
			name: "empty EventData keeps defaults",
			eventXML: `<Event xmlns="http://schemas.microsoft.com/win/2004/08/events/event">
				<System>
					<Provider Name="MsiInstaller"/>
					<EventID>11708</EventID>
				</System>
				<EventData></EventData>
			</Event>`,
			defaultTitle:    "Failed application installation",
			defaultMessage:  "An application installation (MSI) failed",
			expectedTitle:   "Failed application installation",
			expectedMessage: "An application installation (MSI) failed",
		},
		{
			name: "non-standard format keeps default title but sets message",
			eventXML: `<Event xmlns="http://schemas.microsoft.com/win/2004/08/events/event">
				<System>
					<Provider Name="MsiInstaller"/>
					<EventID>11708</EventID>
				</System>
				<EventData>
					<Data>Some other error format without Product prefix</Data>
					<Data>(NULL)</Data>
				</EventData>
			</Event>`,
			defaultTitle:    "Failed application installation",
			defaultMessage:  "An application installation (MSI) failed",
			expectedTitle:   "Failed application installation",
			expectedMessage: "Some other error format without Product prefix",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eventMap, err := parseEventXML([]byte(tt.eventXML))
			require.NoError(t, err)

			payload := &eventPayload{Title: tt.defaultTitle, Message: tt.defaultMessage}
			formatMsiInstallerPayload(payload, eventMap.Map)
			assert.Equal(t, tt.expectedTitle, payload.Title)
			assert.Equal(t, tt.expectedMessage, payload.Message)
		})
	}
}
