// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package client

import (
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
)

func TestMergeDatadogConfig(t *testing.T) {
	t.Run("merges the api key into the base config", func(t *testing.T) {
		params := &agentparams.Params{AgentConfig: "log_level: debug"}
		got, err := mergeDatadogConfig(params, "abc123")
		require.NoError(t, err)
		assert.Contains(t, got, "log_level: debug")
		assert.Contains(t, got, "api_key: abc123")
	})

	t.Run("omits the api key when SkipAPIKeyInConfig is set", func(t *testing.T) {
		params := &agentparams.Params{AgentConfig: "log_level: debug", SkipAPIKeyInConfig: true}
		got, err := mergeDatadogConfig(params, "abc123")
		require.NoError(t, err)
		assert.NotContains(t, got, "api_key")
	})

	t.Run("merges constant extra config entries", func(t *testing.T) {
		params := &agentparams.Params{ExtraAgentConfig: []pulumi.StringInput{pulumi.String("logs_enabled: true")}}
		got, err := mergeDatadogConfig(params, "k")
		require.NoError(t, err)
		assert.Contains(t, got, "logs_enabled: true")
		assert.Contains(t, got, "api_key: k")
	})

	t.Run("rejects computed (pulumi output) extra config", func(t *testing.T) {
		params := &agentparams.Params{ExtraAgentConfig: []pulumi.StringInput{pulumi.Sprintf("tags: [%s]", "team:agent")}}
		_, err := mergeDatadogConfig(params, "k")
		require.Error(t, err)
	})
}
