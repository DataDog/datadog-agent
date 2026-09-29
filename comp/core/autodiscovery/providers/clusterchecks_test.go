// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package providers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestCheckCompatibilityFromConfig(t *testing.T) {
	// configmock.New returns the shared global config within a test, so set
	// and reset the keys explicitly between the cases instead of recreating it.
	cfg := configmock.New(t)
	reset := func() {
		cfg.SetInTest("experimental.clc_runner_checks_include", []string{})
		cfg.SetInTest("experimental.clc_runner_checks_exclude", []string{})
	}
	reset()

	// Nothing set: unrestricted (nil).
	assert.Nil(t, checkCompatibilityFromConfig(cfg))

	// DD_EXPERIMENTAL_CLC_RUNNER_CHECKS_* env vars bind to these keys.
	cfg.SetInTest("experimental.clc_runner_checks_include", []string{"kubernetes_state_core", "orchestrator"})
	cfg.SetInTest("experimental.clc_runner_checks_exclude", []string{"http_check"})
	compat := checkCompatibilityFromConfig(cfg)
	require.NotNil(t, compat)
	assert.Equal(t, []string{"kubernetes_state_core", "orchestrator"}, compat.Include)
	assert.Equal(t, []string{"http_check"}, compat.Exclude)
	reset()

	// Only the experimental exclude key set.
	cfg.SetInTest("experimental.clc_runner_checks_exclude", []string{"kafka_consumer"})
	compat = checkCompatibilityFromConfig(cfg)
	require.NotNil(t, compat)
	assert.Empty(t, compat.Include)
	assert.Equal(t, []string{"kafka_consumer"}, compat.Exclude)
	reset()

	// An include with an empty exclude is a valid claim.
	cfg.SetInTest("experimental.clc_runner_checks_include", []string{"kubernetes_state_core"})
	compat = checkCompatibilityFromConfig(cfg)
	require.NotNil(t, compat)
	assert.Equal(t, []string{"kubernetes_state_core"}, compat.Include)
	assert.Empty(t, compat.Exclude)
}

func TestClusterChecksNewNodeStatus(t *testing.T) {
	// Unrestricted worker: no compat in the status POST.
	c := &ClusterChecksConfigProvider{
		nodeType:    types.NodeTypeNodeAgent,
		checkCompat: nil,
	}
	status := c.newNodeStatus(7)
	assert.EqualValues(t, 7, status.LastChange)
	assert.Equal(t, types.NodeTypeNodeAgent, status.NodeType)
	assert.Nil(t, status.CheckCompatibility)

	// Compat-declaring worker: the compat rides along on every status POST,
	// including the extra-heartbeat one.
	c = &ClusterChecksConfigProvider{
		nodeType: types.NodeTypeCLCRunner,
		checkCompat: &types.CheckCompatibility{
			Include: []string{"kubernetes_state_core"},
		},
	}
	status = c.newNodeStatus(types.ExtraHeartbeatLastChangeValue)
	assert.Equal(t, types.ExtraHeartbeatLastChangeValue, status.LastChange)
	assert.Equal(t, types.NodeTypeCLCRunner, status.NodeType)
	require.NotNil(t, status.CheckCompatibility)
	assert.Equal(t, []string{"kubernetes_state_core"}, status.CheckCompatibility.Include)
}
