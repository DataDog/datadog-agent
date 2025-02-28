// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package aggregator

import (
	_ "embed"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed fixtures/event_json_bytes
var EventData []byte

func TestNewEventPayloads(t *testing.T) {
	t.Run("parseEventPayload empty message should be ignored", func(t *testing.T) {
		payloads, err := ParseEventPayload(api.Payload{Data: []byte(""), Encoding: encodingEmpty})
		require.NoError(t, err)
		require.Empty(t, payloads)
	})

	t.Run("parseEventPayload valid body should parse payloads", func(t *testing.T) {
		payloads, err := ParseEventPayload(api.Payload{Data: EventData, Encoding: encodingZstd})
		require.NoError(t, err)
		require.Len(t, payloads, 1)

		assert.Equal(t, "docker", payloads[0].name())
		expectedTags := []string{"container_id:fe8851e50de127d292822f63ec19d3f86b12a7834270e00a290bd2d87719783c", "container_name:foo", "docker_image:busybox", "image_id:sha256:af47096251092caf59498806ab8d58e8173ecf5a182f024ce9d635b5b4a55d66", "image_name:busybox", "short_image:busybox"}
		assert.Equal(t, expectedTags, payloads[0].GetTags())
		assert.Equal(t, event.AlertTypeError, payloads[0].AlertType)
		assert.Equal(t, "busybox 1 kill 1 die on i-06ac4c9fa7af65518", payloads[0].Title)
	})
}
