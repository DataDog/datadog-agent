// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package haagentimpl

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	haagent "github.com/DataDog/datadog-agent/comp/haagent/def"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
)

var testRCConfigID = "datadog/2/HA_AGENT/config-62345762794c0c0b/65f17d667fb50f8ae28a3c858bdb1be9ea994f20249c119e007c520ac115c807"
var testConfigID = "testConfig01"

func Test_Enabled(t *testing.T) {
	tests := []struct {
		name            string
		configs         map[string]interface{}
		expectedEnabled bool
		expectedError   string
	}{
		{
			name: "enabled",
			configs: map[string]interface{}{
				"ha_agent.enabled": true,
				"config_id":        "foo",
			},
			expectedEnabled: true,
		},
		{
			name: "disabled due to missing config_id",
			configs: map[string]interface{}{
				"ha_agent.enabled": true,
			},
			expectedEnabled: false,
			expectedError:   "HA Agent feature requires config_id to be set",
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
			var b bytes.Buffer
			w := bufio.NewWriter(&b)
			l, err := log.LoggerFromWriterWithMinLevelAndFormat(w, log.WarnLvl, "[%LEVEL] %FuncShort: %Msg")
			assert.Nil(t, err)
			log.SetupLogger(l, "warn")

			haAgent := newTestHaAgentComponent(t, tt.configs, l).Comp.(*haAgentImpl)

			assert.Equal(t, tt.expectedEnabled, haAgent.Enabled())
			haAgent.Enabled()

			l.Close() // We need to first close the logger to avoid a race-cond between seelog and out test when calling w.Flush()
			w.Flush()
			logs := b.String()
			if tt.expectedError != "" {
				assert.Equal(t, 1, strings.Count(logs, tt.expectedError), logs)
			} else {
				assert.Empty(t, logs)
			}
		})
	}
}

func Test_GetConfigID(t *testing.T) {
	agentConfigs := map[string]interface{}{
		"config_id": "my-configID-01",
	}
	haAgent := newTestHaAgentComponent(t, agentConfigs, nil).Comp
	assert.Equal(t, "my-configID-01", haAgent.GetConfigID())
}

func Test_GetState(t *testing.T) {
	agentConfigs := map[string]interface{}{
		"hostname": "my-agent-hostname",
	}
	haAgent := newTestHaAgentComponent(t, agentConfigs, nil).Comp

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
				"config_id":        "foo",
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
			provides := newTestHaAgentComponent(t, tt.configs, nil)
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
				testRCConfigID: {Config: []byte(`{"config_id":"testConfig01","active_agent":"my-agent-hostname"}`)},
			},
			expectedApplyID: testRCConfigID,
			expectedApplyStatus: state.ApplyStatus{
				State: state.ApplyStateAcknowledged,
			},
			expectedAgentState: haagent.Active,
		},
		{
			name:         "successful update with leader NOT matching current agent",
			initialState: haagent.Unknown,
			updates: map[string]state.RawConfig{
				testRCConfigID: {Config: []byte(`{"config_id":"testConfig01","active_agent":"another-agent-hostname"}`)},
			},
			expectedApplyID: testRCConfigID,
			expectedApplyStatus: state.ApplyStatus{
				State: state.ApplyStateAcknowledged,
			},
			expectedAgentState: haagent.Standby,
		},
		{
			name:         "invalid payload",
			initialState: haagent.Unknown,
			updates: map[string]state.RawConfig{
				testRCConfigID: {Config: []byte(`invalid-json`)},
			},
			expectedApplyID: testRCConfigID,
			expectedApplyStatus: state.ApplyStatus{
				State: state.ApplyStateError,
				Error: "error unmarshalling payload",
			},
			expectedAgentState: haagent.Unknown,
		},
		{
			name:         "invalid configID",
			initialState: haagent.Unknown,
			updates: map[string]state.RawConfig{
				testRCConfigID: {Config: []byte(`{"config_id":"invalidConfig","active_agent":"another-agent-hostname"}`)},
			},
			expectedApplyID: testRCConfigID,
			expectedApplyStatus: state.ApplyStatus{
				State: state.ApplyStateError,
				Error: "config_id does not match",
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
				"config_id":        testConfigID,
			}
			agentConfigComponent := fxutil.Test[config.Component](t, fx.Options(
				config.MockModule(),
				fx.Replace(config.MockParams{Overrides: agentConfigs}),
			))

			h := newHaAgentImpl(logmock.New(t), newHaAgentConfigs(agentConfigComponent))

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
		})
	}
}

func Test_haAgentImpl_resetAgentState(t *testing.T) {
	// GIVEN
	haAgent := newTestHaAgentComponent(t, nil, nil)
	haAgentComp := haAgent.Comp.(*haAgentImpl)
	haAgentComp.state.Store(string(haagent.Active))
	require.Equal(t, haagent.Active, haAgentComp.GetState())

	// WHEN
	haAgentComp.resetAgentState()

	// THEN
	assert.Equal(t, haagent.Unknown, haAgentComp.GetState())
}

func Test_IsActive(t *testing.T) {
	agentConfigs := map[string]interface{}{
		"hostname": "my-agent-hostname",
	}
	haAgent := newTestHaAgentComponent(t, agentConfigs, nil).Comp

	haAgent.SetLeader("another-agent")
	assert.False(t, haAgent.IsActive())

	haAgent.SetLeader("my-agent-hostname")
	assert.True(t, haAgent.IsActive())
}
