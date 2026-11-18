// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build (windows || darwin) && test

package logondurationimpl

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildEventPayload_IncludesPayloadVersion(t *testing.T) {
	custom := map[string]interface{}{"existing": "kept"}
	payload, err := buildEventPayload(eventInput{Custom: custom})
	require.NoError(t, err)
	attributes := payload["data"].(map[string]interface{})["attributes"].(map[string]interface{})
	got := attributes["attributes"].(map[string]interface{})["custom"].(map[string]interface{})
	assert.Equal(t, map[string]interface{}{"existing": "kept", "payload_version": 1}, got)
	assert.Equal(t, map[string]interface{}{"existing": "kept"}, custom)

	encoded, err := json.Marshal(payload)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"payload_version":1`)
}

func TestBuildEventPayload_NilCustom(t *testing.T) {
	payload, err := buildEventPayload(eventInput{})
	require.NoError(t, err)
	attributes := payload["data"].(map[string]interface{})["attributes"].(map[string]interface{})
	custom := attributes["attributes"].(map[string]interface{})["custom"]
	assert.Equal(t, map[string]interface{}{"payload_version": 1}, custom)
}
