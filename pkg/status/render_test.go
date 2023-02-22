// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package status

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatStatus(t *testing.T) {
	agentJson, err := os.ReadFile("fixtures/agent_status.json")
	require.NoError(t, err)
	const statusRenderErrors = "Status render errors"

	t.Run("render errors", func(t *testing.T) {
		actual, err := FormatStatus([]byte{})
		require.NoError(t, err)
		assert.Contains(t, actual, statusRenderErrors)
	})

	t.Run("no render errors", func(t *testing.T) {
		actual, err := FormatStatus(agentJson)
		require.NoError(t, err)
		assert.NotContains(t, actual, statusRenderErrors)
	})
}
