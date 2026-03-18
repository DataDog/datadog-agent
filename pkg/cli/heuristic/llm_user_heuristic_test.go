// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package heuristic

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildScorePayloadTemporalSignals(t *testing.T) {
	_ = os.Remove(statePath())
	t.Cleanup(func() {
		_ = os.Remove(statePath())
	})

	now := time.Unix(1700000000, 0).UTC()
	firstPayload, err := BuildScorePayload("agent status", []string{"-j"}, now)
	require.NoError(t, err)
	require.NotNil(t, firstPayload)
	assert.True(t, firstPayload.FirstInvocationInSession)
	assert.True(t, firstPayload.UsedMachineFirstFlags)
	assert.LessOrEqual(t, firstPayload.HeuristicScore, 40)

	secondPayload, err := BuildScorePayload("agent health", []string{}, now.Add(300*time.Millisecond))
	require.NoError(t, err)
	require.NotNil(t, secondPayload)
	require.NotNil(t, secondPayload.InterCommandLatencyMS)
	assert.EqualValues(t, 300, *secondPayload.InterCommandLatencyMS)
	assert.GreaterOrEqual(t, secondPayload.HeuristicScore, 90)
	assert.Contains(t, secondPayload.TriggeredSignals, "complex_diag_sub_500ms")
	assert.Contains(t, secondPayload.TriggeredSignals, "shell_aging_sub_2s")
}

func TestHasMachineFirstFlags(t *testing.T) {
	assert.True(t, hasMachineFirstFlags([]string{"status", "-j"}))
	assert.True(t, hasMachineFirstFlags([]string{"health", "--json"}))
	assert.True(t, hasMachineFirstFlags([]string{"status", "--pretty-json"}))
	assert.False(t, hasMachineFirstFlags([]string{"status", "--verbose"}))
}

func TestHasPipeToFilters(t *testing.T) {
	assert.True(t, hasPipeToFilters(`bash -lc agent status -j | jq '.foo'`))
	assert.True(t, hasPipeToFilters(`bash -lc agent status | grep healthy`))
	assert.True(t, hasPipeToFilters(`bash -lc agent status | yq '.bar'`))
	assert.False(t, hasPipeToFilters(`bash -lc agent status`))
}
