// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package haagentimpl

import (
	"testing"

	haagent "github.com/DataDog/datadog-agent/comp/haagent/def"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
				"enable_metadata_collection": true,
				"inventories_enabled":        true,
				"ha_agent.enabled":           true,
			},
			expectedEnabled: true,
		},
		{
			name: "disabled",
			configs: map[string]interface{}{
				"enable_metadata_collection": true,
				"inventories_enabled":        true,
				"ha_agent.enabled":           false,
			},
			expectedEnabled: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provides, deps := newTestHaAgentComponent(t, tt.configs)
			haAgent := provides.Comp
			assert.Equal(t, tt.expectedEnabled, haAgent.Enabled())
			assert.Equal(t, tt.expectedEnabled, deps.InventoryAgent.Get()["ha_agent_enabled"])
		})
	}
}

func Test_GetGroup(t *testing.T) {
	agentConfigs := map[string]interface{}{
		"ha_agent.group": "my-group-01",
	}
	provides, _ := newTestHaAgentComponent(t, agentConfigs)
	haAgent := provides.Comp
	assert.Equal(t, "my-group-01", haAgent.GetGroup())
}

func Test_GetState(t *testing.T) {
	agentConfigs := map[string]interface{}{
		"hostname": "my-agent-hostname",
	}
	provides, _ := newTestHaAgentComponent(t, agentConfigs)
	haAgent := provides.Comp

	assert.Equal(t, haagent.Unknown, haAgent.GetState())

	haAgent.SetLeader("another-agent")
	assert.Equal(t, haagent.Standby, haAgent.GetState())

	haAgent.SetLeader("my-agent-hostname")
	assert.Equal(t, haagent.Active, haAgent.GetState())
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
			provides, _ := newTestHaAgentComponent(t, tt.configs)
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
		initialState        haagent.State
		updates             map[string]state.RawConfig
		expectedApplyID     string
		expectedApplyStatus state.ApplyStatus
		expectedAgentState  haagent.State
	}{
		{
			name:         "successful update with leader matching current agent",
			initialState: haagent.Unknown,
			updates: map[string]state.RawConfig{
				testConfigID: {Config: []byte(`{"group":"testGroup01","leader":"my-agent-hostname"}`)},
			},
			expectedApplyID: testConfigID,
			expectedApplyStatus: state.ApplyStatus{
				State: state.ApplyStateAcknowledged,
			},
			expectedAgentState: haagent.Active,
		},
		{
			name:         "successful update with leader NOT matching current agent",
			initialState: haagent.Unknown,
			updates: map[string]state.RawConfig{
				testConfigID: {Config: []byte(`{"group":"testGroup01","leader":"another-agent-hostname"}`)},
			},
			expectedApplyID: testConfigID,
			expectedApplyStatus: state.ApplyStatus{
				State: state.ApplyStateAcknowledged,
			},
			expectedAgentState: haagent.Standby,
		},
		{
			name:         "invalid payload",
			initialState: haagent.Unknown,
			updates: map[string]state.RawConfig{
				testConfigID: {Config: []byte(`invalid-json`)},
			},
			expectedApplyID: testConfigID,
			expectedApplyStatus: state.ApplyStatus{
				State: state.ApplyStateError,
				Error: "error unmarshalling payload",
			},
			expectedAgentState: haagent.Unknown,
		},
		{
			name:         "invalid group",
			initialState: haagent.Unknown,
			updates: map[string]state.RawConfig{
				testConfigID: {Config: []byte(`{"group":"invalidGroup","leader":"another-agent-hostname"}`)},
			},
			expectedApplyID: testConfigID,
			expectedApplyStatus: state.ApplyStatus{
				State: state.ApplyStateError,
				Error: "group does not match",
			},
			expectedAgentState: haagent.Unknown,
		},
		{
			name:                "empty update",
			initialState:        haagent.Active,
			updates:             map[string]state.RawConfig{},
			expectedApplyID:     "",
			expectedApplyStatus: state.ApplyStatus{},
			expectedAgentState:  haagent.Unknown,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agentConfigs := map[string]interface{}{
				"hostname":         "my-agent-hostname",
				"ha_agent.enabled": true,
				"ha_agent.group":   testGroup,
			}

			provides, deps := newTestHaAgentComponent(t, agentConfigs)
			h := provides.Comp.(*haAgentImpl)

			if tt.initialState != "" {
				h.state.Store(string(tt.initialState))
			}

			var applyID string
			var applyStatus state.ApplyStatus
			applyFunc := func(id string, status state.ApplyStatus) {
				applyID = id
				applyStatus = status
			}
			h.onHaAgentUpdate(tt.updates, applyFunc)
			assert.Equal(t, tt.expectedApplyID, applyID)
			assert.Equal(t, tt.expectedApplyStatus, applyStatus)
			assert.Equal(t, tt.expectedAgentState, h.GetState())
			assert.Equal(t, deps.InventoryAgent.Get()["ha_agent_state"], string(h.GetState()))
		})
	}
}

func Test_haAgentImpl_ShouldRunIntegration(t *testing.T) {
	testAgentHostname := "my-agent-hostname"
	tests := []struct {
		name                       string
		leader                     string
		agentConfigs               map[string]interface{}
		expectShouldRunIntegration map[string]bool
	}{
		{
			name: "ha agent enabled and agent is leader",
			// should run HA-integrations
			// should run "non HA integrations"
			agentConfigs: map[string]interface{}{
				"hostname":         testAgentHostname,
				"ha_agent.enabled": true,
				"ha_agent.group":   testGroup,
			},
			leader: testAgentHostname,
			expectShouldRunIntegration: map[string]bool{
				"snmp":                true,
				"cisco_aci":           true,
				"cisco_sdwan":         true,
				"network_path":        true,
				"unknown_integration": true,
				"cpu":                 true,
			},
		},
		{
			name: "ha agent enabled and agent is not active",
			// should skip HA-integrations
			// should run "non HA integrations"
			agentConfigs: map[string]interface{}{
				"hostname":         testAgentHostname,
				"ha_agent.enabled": true,
				"ha_agent.group":   testGroup,
			},
			leader: "another-agent-is-active",
			expectShouldRunIntegration: map[string]bool{
				"snmp":                false,
				"cisco_aci":           false,
				"cisco_sdwan":         false,
				"network_path":        false,
				"unknown_integration": true,
				"cpu":                 true,
			},
		},
		{
			name: "ha agent not enabled",
			// should run all integrations
			agentConfigs: map[string]interface{}{
				"hostname":         testAgentHostname,
				"ha_agent.enabled": false,
				"ha_agent.group":   testGroup,
			},
			leader: testAgentHostname,
			expectShouldRunIntegration: map[string]bool{
				"snmp":                true,
				"cisco_aci":           true,
				"cisco_sdwan":         true,
				"network_path":        true,
				"unknown_integration": true,
				"cpu":                 true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			haAgent, _ := newTestHaAgentComponent(t, tt.agentConfigs)
			haAgent.Comp.SetLeader(tt.leader)

			for integrationName, shouldRun := range tt.expectShouldRunIntegration {
				assert.Equalf(t, shouldRun, haAgent.Comp.ShouldRunIntegration(integrationName), "fail for integration: "+integrationName)
			}
		})
	}
}

func Test_haAgentImpl_resetAgentState(t *testing.T) {
	// GIVEN
	haAgent, _ := newTestHaAgentComponent(t, nil)
	haAgentComp := haAgent.Comp.(*haAgentImpl)
	haAgentComp.state.Store(string(haagent.Active))
	require.Equal(t, haagent.Active, haAgentComp.GetState())

	// WHEN
	haAgentComp.resetAgentState()

	// THEN
	assert.Equal(t, haagent.Unknown, haAgentComp.GetState())
}
