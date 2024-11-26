// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package haagentimpl

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
)

var testConfigID = "datadog/2/HA_AGENT/group-62345762794c0c0b/65f17d667fb50f8ae28a3c858bdb1be9ea994f20249c119e007c520ac115c807"
var testGroup = "testGroup01"

func Test_Enabled(t *testing.T) {
	tests := []struct {
		name            string
		configs         map[string]interface{}
		expectedEnabled bool
	}{
		{
			name: "enabled",
			configs: map[string]interface{}{
				"ha_agent.enabled": true,
			},
			expectedEnabled: true,
		},
		{
			name: "disabled",
			configs: map[string]interface{}{
				"ha_agent.enabled": false,
			},
			expectedEnabled: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			haAgent := newTestHaAgentComponent(t, tt.configs).Comp
			assert.Equal(t, tt.expectedEnabled, haAgent.Enabled())
		})
	}
}

func Test_GetGroup(t *testing.T) {
	agentConfigs := map[string]interface{}{
		"ha_agent.group": "my-group-01",
	}
	haAgent := newTestHaAgentComponent(t, agentConfigs).Comp
	assert.Equal(t, "my-group-01", haAgent.GetGroup())
}

func Test_IsLeader_SetLeader(t *testing.T) {
	agentConfigs := map[string]interface{}{
		"hostname": "my-agent-hostname",
	}
	haAgent := newTestHaAgentComponent(t, agentConfigs).Comp

	haAgent.SetLeader("another-agent")
	assert.False(t, haAgent.IsLeader())

	haAgent.SetLeader("my-agent-hostname")
	assert.True(t, haAgent.IsLeader())
}

func Test_IsHaIntegration(t *testing.T) {
	agentConfigs := map[string]interface{}{
		"hostname":         "my-agent-hostname",
		"ha_agent.enabled": true,
		"ha_agent.group":   testGroup,
	}
	haAgent := newTestHaAgentComponent(t, agentConfigs)

	assert.True(t, haAgent.Comp.IsHaIntegration("snmp"))
	assert.False(t, haAgent.Comp.IsHaIntegration("unknown_integration"))
	assert.False(t, haAgent.Comp.IsHaIntegration("cpu"))
}

func Test_RCListener(t *testing.T) {
	tests := []struct {
		name             string
		configs          map[string]interface{}
		expectRCListener bool
	}{
		{
			name: "enabled",
			configs: map[string]interface{}{
				"ha_agent.enabled": true,
			},
			expectRCListener: true,
		},
		{
			name: "disabled",
			configs: map[string]interface{}{
				"ha_agent.enabled": false,
			},
			expectRCListener: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provides := newTestHaAgentComponent(t, tt.configs)
			if tt.expectRCListener {
				assert.NotNil(t, provides.RCListener.ListenerProvider)
			} else {
				assert.Nil(t, provides.RCListener.ListenerProvider)
			}
		})
	}
}

func Test_haAgentImpl_onHaAgentUpdate(t *testing.T) {

	tests := []struct {
		name                string
		updates             map[string]state.RawConfig
		expectedApplyID     string
		expectedApplyStatus state.ApplyStatus
	}{
		{
			name: "successful update",
			updates: map[string]state.RawConfig{
				testConfigID: {Config: []byte(`{"group":"testGroup01","leader":"ha-agent1"}`)},
			},
			expectedApplyID: testConfigID,
			expectedApplyStatus: state.ApplyStatus{
				State: state.ApplyStateUnacknowledged,
			},
		},
		{
			name: "invalid payload",
			updates: map[string]state.RawConfig{
				testConfigID: {Config: []byte(`invalid-json`)},
			},
			expectedApplyID: testConfigID,
			expectedApplyStatus: state.ApplyStatus{
				State: state.ApplyStateError,
				Error: "error unmarshalling payload",
			},
		},
		{
			name: "invalid group",
			updates: map[string]state.RawConfig{
				testConfigID: {Config: []byte(`{"group":"invalidGroup","leader":"ha-agent1"}`)},
			},
			expectedApplyID: testConfigID,
			expectedApplyStatus: state.ApplyStatus{
				State: state.ApplyStateError,
				Error: "group does not match",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agentConfigs := map[string]interface{}{
				"hostname":         "my-agent-hostname",
				"ha_agent.enabled": true,
				"ha_agent.group":   testGroup,
			}
			agentConfigComponent := fxutil.Test[config.Component](t, fx.Options(
				config.MockModule(),
				fx.Replace(config.MockParams{Overrides: agentConfigs}),
			))

			h := newHaAgentImpl(logmock.New(t), newHaAgentConfigs(agentConfigComponent))

			var applyID string
			var applyStatus state.ApplyStatus
			applyFunc := func(id string, status state.ApplyStatus) {
				applyID = id
				applyStatus = status
			}
			h.onHaAgentUpdate(tt.updates, applyFunc)
			assert.Equal(t, tt.expectedApplyID, applyID)
			assert.Equal(t, tt.expectedApplyStatus, applyStatus)
		})
	}
}
