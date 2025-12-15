// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package notableeventsimpl

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	logscompressionmock "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestSubmitter_DrainChannelAndPayloadFormat(t *testing.T) {
	// Create noop forwarder
	hostname := fxutil.Test[hostnameinterface.Component](t, hostnameimpl.MockModule())
	compression := fxutil.Test[logscompression.Component](t, logscompressionmock.MockModule())
	forwarder := eventplatformimpl.NewNoopEventPlatformForwarder(hostname, compression)

	// Create event channel
	eventChan := make(chan eventPayload)

	// Create submitter with forwarder
	sub := newSubmitter(forwarder, eventChan, hostname)

	// Start submitter
	sub.start()

	// Send multiple test events
	numEvents := 5
	for i := 0; i < numEvents; i++ {
		eventChan <- eventPayload{
			Channel: "System",
			EventID: uint(7040 + i),
		}
	}

	// Stop submitter (close channel and wait for drain)
	close(eventChan)
	sub.stop()

	// Verify sent messages
	sentMessages := forwarder.Purge()
	eventsV2Messages := sentMessages[eventplatform.EventTypeEventManagement]
	require.Len(t, eventsV2Messages, numEvents, "Expected %d events to be sent", numEvents)

	// Verify structure for each event
	for i := 0; i < numEvents; i++ {
		var payload map[string]interface{}
		err := json.Unmarshal(eventsV2Messages[i].GetContent(), &payload)
		require.NoError(t, err, "Payload should be valid JSON")

		// Verify required fields
		// https://docs.datadoghq.com/api/latest/events/?code-lang=go#post-an-event
		data, ok := payload["data"].(map[string]interface{})
		require.True(t, ok, "Payload should have 'data' field")
		assert.Equal(t, "event", data["type"], "Event type should be 'event'")
		attributes, ok := data["attributes"].(map[string]interface{})
		require.True(t, ok, "Data should have 'attributes' field")
		_, ok = attributes["title"].(string)
		require.True(t, ok, "Attributes should have 'title' field")
		_, ok = attributes["host"].(string)
		require.True(t, ok, "Attributes should have 'host' field")
		assert.Equal(t, "alert", attributes["category"], "Category should be 'alert'")
		nestedAttrs, ok := attributes["attributes"].(map[string]interface{})
		require.True(t, ok, "Attributes should have nested 'attributes' field")
		assert.Equal(t, "error", nestedAttrs["status"], "Status should be 'error'")
	}
}
