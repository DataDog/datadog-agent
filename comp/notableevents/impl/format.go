// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package notableeventsimpl

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/logs/util/windowsevent"
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

// formatAppCrashPayload customizes the payload for application crash events (Application Error / Event ID 1000).
// Extracts the AppName from EventData to create a dynamic title.
func formatAppCrashPayload(payload *eventPayload, eventData map[string]interface{}) {
	if data := getEventDataMap(eventData); data != nil {
		if appName, ok := data["AppName"].(string); ok {
			payload.Title = fmt.Sprintf("Application crash: %s", appName)
		}
	}
}

// formatAppHangPayload customizes the payload for application hang events (Application Hang / Event ID 1002).
// Extracts the AppName from EventData to create a dynamic title.
func formatAppHangPayload(payload *eventPayload, eventData map[string]interface{}) {
	if data := getEventDataMap(eventData); data != nil {
		if appName, ok := data["AppName"].(string); ok {
			payload.Title = fmt.Sprintf("Application hang: %s", appName)
		}
	}
}

// formatMsiInstallerPayload customizes the payload for MSI installer failure events (MsiInstaller / Event ID 11708).
// Parses the first Data element which has format: "Product: {Name} -- {Error message}"
// Sets the title to include the product name and the message to the full error text.
func formatMsiInstallerPayload(payload *eventPayload, eventData map[string]interface{}) {
	if dataList := getEventDataList(eventData); len(dataList) > 0 {
		if text, ok := dataList[0].(string); ok {
			payload.Message = text
			// Parse "Product: Name -- Error" format
			// Example: Product: Slack (Machine - MSI) -- Error 1714. The older version of Slack (Machine - MSI) cannot be removed. Contact your technical support group. System Error 1612.
			// Example: Product: Datadog Agent -- Automatic downgrades are not supported. Uninstall the current version, and then reinstall the desired version.
			if strings.HasPrefix(text, "Product: ") {
				parts := strings.SplitN(text[9:], " -- ", 2)
				if len(parts) > 0 {
					payload.Title = fmt.Sprintf("Failed application installation: %s", parts[0])
				}
			}
		}
	}
}
