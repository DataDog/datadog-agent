// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package notableeventsimpl

import (
	"fmt"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/logs/util/windowsevent"
	"golang.org/x/sys/windows"
)

// parseEventXML parses Windows Event Log XML into a map structure.
func parseEventXML(xmlData []byte) (*windowsevent.Map, error) {
	return windowsevent.NewMapXMLWithOptions(xmlData, windowsevent.TransformOptions{
		FormatEventData:  true,
		FormatBinaryData: false, // Skip buggy binary transform
		NormalizeEventID: true,
	})
}

// PayloadFormatter customizes the event payload using data from the parsed event XML.
// The payload is pre-populated with default values; the formatter can modify any fields.
type PayloadFormatter func(payload *eventPayload, eventData map[string]interface{})

// getEventDataMap extracts the EventData.Data map from the parsed event (for named data fields).
// Returns nil if the path doesn't exist or the data is not a map.
func getEventDataMap(eventMap map[string]interface{}) map[string]interface{} {
	event, ok := eventMap["Event"].(map[string]interface{})
	if !ok {
		return nil
	}
	eventData, ok := event["EventData"].(map[string]interface{})
	if !ok {
		return nil
	}
	data, ok := eventData["Data"].(map[string]interface{})
	if !ok {
		return nil
	}
	return data
}

// getEventDataList extracts EventData.Data as a list (for unnamed data fields).
// Returns nil if the path doesn't exist or the data is not a list.
func getEventDataList(eventMap map[string]interface{}) []interface{} {
	event, ok := eventMap["Event"].(map[string]interface{})
	if !ok {
		return nil
	}
	eventData, ok := event["EventData"].(map[string]interface{})
	if !ok {
		return nil
	}
	data, ok := eventData["Data"].([]interface{})
	if !ok {
		return nil
	}
	return data
}

// formatMsiExitCode converts an MSI exit code string to a human-readable error message.
// Uses windows.Errno to get the system error message for known MSI error codes.
// Reference: https://learn.microsoft.com/en-us/windows/win32/msi/error-codes
func formatMsiExitCode(exitCodeStr string) string {
	code, err := strconv.Atoi(exitCodeStr)
	if err != nil {
		return exitCodeStr
	}
	errno := windows.Errno(code)
	errMsg := errno.Error()
	return fmt.Sprintf("%s (%s)", exitCodeStr, errMsg)
}

// formatAppCrashPayload customizes the payload for application crash events (Application Error / Event ID 1000).
// Extracts the AppName from EventData to create a dynamic title.
// Supports both Server 2025 (named map fields) and Server 2022 (positional list fields) formats.
// List format: [0]=AppName, [1]=AppVersion, [2]=AppTimestamp, [3]=FaultModuleName, ...
func formatAppCrashPayload(payload *eventPayload, eventData map[string]interface{}) {
	var appName string

	// Try named map fields first (Server 2025)
	if data := getEventDataMap(eventData); data != nil {
		appName, _ = data["AppName"].(string)
	}

	// Fall back to positional list fields (Server 2022)
	if appName == "" {
		if dataList := getEventDataList(eventData); len(dataList) > 0 {
			appName, _ = dataList[0].(string)
		}
	}

	if appName != "" {
		payload.Title = "Application crash: " + appName
	}
}

// formatAppHangPayload customizes the payload for application hang events (Application Hang / Event ID 1002).
// Extracts the AppName from EventData to create a dynamic title.
// Supports both Server 2025 (named map fields) and Server 2022 (positional list fields) formats.
// List format: [0]=AppName, [1]=AppVersion, ...
func formatAppHangPayload(payload *eventPayload, eventData map[string]interface{}) {
	var appName string

	// Try named map fields first (Server 2025)
	if data := getEventDataMap(eventData); data != nil {
		appName, _ = data["AppName"].(string)
	}

	// Fall back to positional list fields (Server 2022)
	if appName == "" {
		if dataList := getEventDataList(eventData); len(dataList) > 0 {
			appName, _ = dataList[0].(string)
		}
	}

	if appName != "" {
		payload.Title = "Application hang: " + appName
	}
}

// formatWindowsUpdateFailedPayload customizes the payload for failed Windows Update events
// (Microsoft-Windows-WindowsUpdateClient / Event ID 20).
// Extracts updateTitle for the title and includes errorCode in the message.
func formatWindowsUpdateFailedPayload(payload *eventPayload, eventData map[string]interface{}) {
	if data := getEventDataMap(eventData); data != nil {
		updateTitle, _ := data["updateTitle"].(string)
		errorCode, _ := data["errorCode"].(string)

		if updateTitle != "" {
			payload.Title = "Failed Windows update: " + updateTitle
		}
		if updateTitle != "" && errorCode != "" {
			payload.Message = fmt.Sprintf("Installation of %s failed with error %s", updateTitle, errorCode)
		} else if errorCode != "" {
			payload.Message = "Windows Update failed with error " + errorCode
		}
	}
}

// formatMsiInstaller1033Payload customizes the payload for MSI installer result events (MsiInstaller / Event ID 1033).
// Data format: [0]=ProductName, [1]=ProductVersion, [2]=Language, [3]=ExitCode, [4]=Manufacturer, [5]=(NULL)
// Note: The query filters out successful installations (exit code 0) at the XPath level.
func formatMsiInstaller1033Payload(payload *eventPayload, eventData map[string]interface{}) {
	dataList := getEventDataList(eventData)
	if len(dataList) < 4 {
		return
	}

	// Extract product name, version, and exit code
	productName, _ := dataList[0].(string)
	productVersion, _ := dataList[1].(string)
	exitCode, _ := dataList[3].(string)

	if productName != "" {
		if productVersion != "" {
			payload.Title = fmt.Sprintf("Failed application installation: %s %s", productName, productVersion)
		} else {
			payload.Title = "Failed application installation: " + productName
		}
		payload.Message = fmt.Sprintf("Installation of %s failed with exit code %s", productName, formatMsiExitCode(exitCode))
	}
}

// formatMsiInstaller1034Payload customizes the payload for MSI uninstall result events (MsiInstaller / Event ID 1034).
// Data format: [0]=ProductName, [1]=ProductVersion, [2]=Language, [3]=ExitCode, [4]=Manufacturer, [5]=(NULL)
// Note: The query filters out successful removals (exit code 0) at the XPath level.
func formatMsiInstaller1034Payload(payload *eventPayload, eventData map[string]interface{}) {
	dataList := getEventDataList(eventData)
	if len(dataList) < 4 {
		return
	}

	// Extract product name, version, and exit code
	productName, _ := dataList[0].(string)
	productVersion, _ := dataList[1].(string)
	exitCode, _ := dataList[3].(string)

	if productName != "" {
		if productVersion != "" {
			payload.Title = fmt.Sprintf("Failed application removal: %s %s", productName, productVersion)
		} else {
			payload.Title = "Failed application removal: " + productName
		}
		payload.Message = fmt.Sprintf("Removal of %s failed with exit code %s", productName, formatMsiExitCode(exitCode))
	}
}

// formatUnexpectedRebootPayload customizes the payload for unexpected reboot events
// (Microsoft-Windows-Kernel-Power / Event ID 41).
// Distinguishes between regular unexpected reboots (BugcheckCode=0) and
// bugcheck-caused reboots (BugcheckCode!=0).
func formatUnexpectedRebootPayload(payload *eventPayload, eventData map[string]interface{}) {
	data := getEventDataMap(eventData)
	if data == nil {
		return
	}

	bugcheckCodeStr, _ := data["BugcheckCode"].(string)
	bugcheckCode, _ := strconv.Atoi(bugcheckCodeStr)

	if bugcheckCode != 0 {
		payload.EventType = "System crash"
		payload.Title = fmt.Sprintf("System crash (bugcheck code:0x%X)", bugcheckCode)
		payload.Message = fmt.Sprintf("The system crashed with bugcheck code 0x%X", bugcheckCode)
	}
	// If BugcheckCode=0, keep the default "Unexpected reboot" values
}
