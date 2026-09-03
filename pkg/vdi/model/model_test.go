// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSessionDoesNotRequireConnections(t *testing.T) {
	session := Session{ID: "3", Protocol: "rdp", User: `DOMAIN\user`, State: "active"}

	payload, err := json.Marshal(session)
	require.NoError(t, err)
	require.NotContains(t, string(payload), "connections")

	var decoded Session
	require.NoError(t, json.Unmarshal(payload, &decoded))
	require.Equal(t, session, decoded)
	require.Empty(t, decoded.Connections)
}
