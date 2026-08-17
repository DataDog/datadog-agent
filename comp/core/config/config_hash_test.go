// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/model"
)

func TestGetHashStableWithoutChanges(t *testing.T) {
	config := NewMock(t)

	h1 := config.GetHash()
	h2 := config.GetHash()

	require.NotEmpty(t, h1)
	require.Equal(t, h1, h2)
}

func TestGetHashChangesAfterSet(t *testing.T) {
	config := NewMock(t)

	before := config.GetHash()
	config.Set("app_key", "changed-value", model.SourceAgentRuntime)
	after := config.GetHash()

	require.NotEqual(t, before, after)
}

func TestGetHashChangesAfterRemoteConfigSet(t *testing.T) {
	config := NewMock(t)

	before := config.GetHash()
	config.Set("apm_config.max_traces_per_second", 42, model.SourceRC)
	after := config.GetHash()

	require.NotEqual(t, before, after)
}

func TestGetHashIsFunctionOfContentNotHistory(t *testing.T) {
	overrides := map[string]interface{}{
		"app_key": "abc1234",
		"dd_url":  "https://example.com",
	}

	config1 := NewMockWithOverrides(t, overrides)
	config2 := NewMockWithOverrides(t, overrides)

	// config2 takes a different path to reach the same effective settings,
	// which bumps its sequence ID further than config1's.
	config2.Set("dd_url", "https://other.example.com", model.SourceAgentRuntime)
	config2.Set("dd_url", "https://example.com", model.SourceAgentRuntime)

	require.Equal(t, config1.GetHash(), config2.GetHash())
}
