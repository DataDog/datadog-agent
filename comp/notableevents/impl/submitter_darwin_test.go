// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package notableeventsimpl

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
)

// TestSubmitterMacOSDiagnosticReportPayload verifies the filtered macOS payload survives envelope serialization.
func TestSubmitterMacOSDiagnosticReportPayload(t *testing.T) {
	const exactInteger = "9007199254740993"
	forwarder, hostname := newTestEventForwarder(t)
	eventChan := make(chan eventPayload, 1)
	submitter := newSubmitter(forwarder, eventChan, hostname)
	submitter.start()
	event := testDarwinEvent()
	event.Custom["macos_diagnostic_report"].(map[string]interface{})["report"].(map[string]interface{})["pid"] = json.Number(exactInteger)
	completion := make(chan error, 1)
	eventChan <- eventPayload{
		Timestamp:  event.Timestamp,
		EventType:  event.EventType,
		Title:      event.Title,
		Message:    event.Message,
		Custom:     event.Custom,
		completion: completion,
	}
	close(eventChan)
	submitter.stop()
	require.NoError(t, <-completion)

	messages := forwarder.Purge()[eventplatform.EventTypeEventManagement]
	require.Len(t, messages, 1)
	content := messages[0].GetContent()
	assert.Contains(t, string(content), `"pid":`+exactInteger)
	var envelope map[string]interface{}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.UseNumber()
	require.NoError(t, decoder.Decode(&envelope))

	requireMap := func(parent map[string]interface{}, key string) map[string]interface{} {
		value, ok := parent[key].(map[string]interface{})
		require.True(t, ok, "%s should be an object", key)
		return value
	}
	data := requireMap(envelope, "data")
	attributes := requireMap(data, "attributes")
	nestedAttributes := requireMap(attributes, "attributes")
	custom := requireMap(nestedAttributes, "custom")
	macOSPayload := requireMap(custom, "macos_diagnostic_report")
	source := requireMap(macOSPayload, "report")

	assert.Equal(t, "incident-1", macOSPayload["incident_id"])
	assert.Equal(t, "user", macOSPayload["scope"])
	assert.Equal(t, "ExampleApp", source["procName"])
	assert.Equal(t, json.Number(exactInteger), source["pid"])
	assert.NotContains(t, source, "storeInfo")
}
