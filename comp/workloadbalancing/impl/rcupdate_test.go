// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package workloadbalancingimpl

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	haagentmock "github.com/DataDog/datadog-agent/comp/haagent/mock"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

const testRCConfigPath = "datadog/2/HA_AGENT/config-workload-balancing/1"

// erroringHostname always fails to resolve, to exercise onWorkloadBalancingUpdate's early return.
type erroringHostname struct{}

func (erroringHostname) Get(context.Context) (string, error) { return "", errors.New("boom") }
func (erroringHostname) GetWithProvider(context.Context) (hostnameinterface.Data, error) {
	return hostnameinterface.Data{}, errors.New("boom")
}
func (erroringHostname) GetSafe(context.Context) string { return "unknown host" }

func newTestWorkloadBalancingImplWithHaAgent(t *testing.T, haAgentEnabled bool) *workloadBalancingImpl {
	haAgent := haagentmock.NewMockHaAgent().(haagentmock.Component)
	haAgent.SetEnabled(haAgentEnabled)

	agentConfigs := map[string]interface{}{
		"hostname":                         "my-agent-hostname",
		"agent_workload_balancing.enabled": true,
	}
	provides := newTestWorkloadBalancingComponent(t, agentConfigs, haAgent, nil)
	return provides.Comp.(*workloadBalancingImpl)
}

func Test_onWorkloadBalancingUpdate(t *testing.T) {
	tests := []struct {
		name               string
		haAgentEnabled     bool
		initialGroups      map[string]groupState
		updates            map[string]state.RawConfig
		expectApplyID      string
		expectApplyStatus  state.ApplyStatus
		expectApplyCalled  bool
		expectGroupID      string
		expectGroupState   groupState
		expectGroupPresent bool
	}{
		{
			name:               "valid assignment, this agent is active",
			updates:            map[string]state.RawConfig{testRCConfigPath: {Config: []byte(`{"type":"workload_balancing","workload_balancing_group_id":"group-01","active_agent":"my-agent-hostname"}`)}},
			expectApplyID:      testRCConfigPath,
			expectApplyStatus:  state.ApplyStatus{State: state.ApplyStateAcknowledged},
			expectApplyCalled:  true,
			expectGroupID:      "group-01",
			expectGroupState:   groupStateActive,
			expectGroupPresent: true,
		},
		{
			name:               "valid assignment, this agent is standby",
			updates:            map[string]state.RawConfig{testRCConfigPath: {Config: []byte(`{"type":"workload_balancing","workload_balancing_group_id":"group-01","active_agent":"another-agent-hostname"}`)}},
			expectApplyID:      testRCConfigPath,
			expectApplyStatus:  state.ApplyStatus{State: state.ApplyStateAcknowledged},
			expectApplyCalled:  true,
			expectGroupID:      "group-01",
			expectGroupState:   groupStateStandby,
			expectGroupPresent: true,
		},
		{
			name:               "present but malformed document keeps the previous assignment and reports an error",
			initialGroups:      map[string]groupState{"group-01": groupStateActive},
			updates:            map[string]state.RawConfig{testRCConfigPath: {Config: []byte(`{"type":"workload_balancing","workload_balancing_group_id":"group-01","active_agent":123}`)}},
			expectApplyID:      testRCConfigPath,
			expectApplyStatus:  state.ApplyStatus{State: state.ApplyStateError, Error: "error unmarshalling payload"},
			expectApplyCalled:  true,
			expectGroupID:      "group-01",
			expectGroupState:   groupStateActive,
			expectGroupPresent: true,
		},
		{
			// The group ID itself is unrecoverable, so there is no group to attribute the
			// previous assignment to; group-01's entry from a prior poll cannot be kept and
			// is dropped (reverting to unmanaged, which still runs -- duplicate over gap).
			name:              "malformed document with an unrecoverable group ID reports an error",
			initialGroups:     map[string]groupState{"group-01": groupStateActive},
			updates:           map[string]state.RawConfig{testRCConfigPath: {Config: []byte(`{"type":"workload_balancing","workload_balancing_group_id":123,"active_agent":"x"}`)}},
			expectApplyID:     testRCConfigPath,
			expectApplyStatus: state.ApplyStatus{State: state.ApplyStateError, Error: "error unmarshalling payload"},
			expectApplyCalled: true,
		},
		{
			name:              "genuinely malformed document, HA Agent enabled, left for HA Agent",
			haAgentEnabled:    true,
			updates:           map[string]state.RawConfig{testRCConfigPath: {Config: []byte(`invalid-json`)}},
			expectApplyCalled: false,
		},
		{
			name:              "genuinely malformed document, HA Agent disabled, reported as fallback owner",
			updates:           map[string]state.RawConfig{testRCConfigPath: {Config: []byte(`invalid-json`)}},
			expectApplyID:     testRCConfigPath,
			expectApplyStatus: state.ApplyStatus{State: state.ApplyStateError, Error: "not a workload balancing document"},
			expectApplyCalled: true,
		},
		{
			name:              "HA Agent shaped document, HA Agent enabled, left for HA Agent",
			haAgentEnabled:    true,
			updates:           map[string]state.RawConfig{testRCConfigPath: {Config: []byte(`{"config_id":"testConfig01","active_agent":"my-agent-hostname"}`)}},
			expectApplyCalled: false,
		},
		{
			name:              "HA Agent shaped document, HA Agent disabled, reported as fallback owner",
			updates:           map[string]state.RawConfig{testRCConfigPath: {Config: []byte(`{"config_id":"testConfig01","active_agent":"my-agent-hostname"}`)}},
			expectApplyID:     testRCConfigPath,
			expectApplyStatus: state.ApplyStatus{State: state.ApplyStateError, Error: "not a workload balancing document"},
			expectApplyCalled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newTestWorkloadBalancingImplWithHaAgent(t, tt.haAgentEnabled)
			if tt.initialGroups != nil {
				w.groups = tt.initialGroups
			}

			var applyID string
			var applyStatus state.ApplyStatus
			applyCalled := false
			applyFunc := func(id string, status state.ApplyStatus) {
				applyID = id
				applyStatus = status
				applyCalled = true
			}

			w.onWorkloadBalancingUpdate(tt.updates, applyFunc)

			assert.Equal(t, tt.expectApplyCalled, applyCalled)
			if tt.expectApplyCalled {
				assert.Equal(t, tt.expectApplyID, applyID)
				assert.Equal(t, tt.expectApplyStatus, applyStatus)
			}
			if tt.expectGroupPresent {
				w.mu.RLock()
				got, ok := w.groups[tt.expectGroupID]
				w.mu.RUnlock()
				assert.True(t, ok)
				assert.Equal(t, tt.expectGroupState, got)
			}
		})
	}
}

func Test_onWorkloadBalancingUpdate_hostnameErrorSkipsTheUpdate(t *testing.T) {
	haAgent := haagentmock.NewMockHaAgent().(haagentmock.Component)
	haAgent.SetEnabled(false)

	w := newWorkloadBalancingImpl(logmock.New(t), erroringHostname{}, haAgent, &workloadBalancingConfigs{enabled: true})

	applyCalled := false
	w.onWorkloadBalancingUpdate(map[string]state.RawConfig{
		testRCConfigPath: {Config: []byte(`{"type":"workload_balancing","workload_balancing_group_id":"group-01","active_agent":"my-agent-hostname"}`)},
	}, func(string, state.ApplyStatus) { applyCalled = true })

	assert.False(t, applyCalled)
	assert.True(t, w.IsGroupActive("group-01"))
}

func Test_onWorkloadBalancingUpdate_unrecoverableGroupIDClearsThatGroup(t *testing.T) {
	w := newTestWorkloadBalancingImplWithHaAgent(t, true)
	w.groups = map[string]groupState{"group-01": groupStateStandby}

	w.onWorkloadBalancingUpdate(map[string]state.RawConfig{
		testRCConfigPath: {Config: []byte(`{"type":"workload_balancing","workload_balancing_group_id":123,"active_agent":"x"}`)},
	}, func(string, state.ApplyStatus) {})

	// Nothing in the batch identified group-01, so its prior assignment cannot be kept and
	// it reverts to unmanaged -- which still runs, the safe direction.
	assert.True(t, w.IsGroupActive("group-01"))
}

func Test_onWorkloadBalancingUpdate_localFlagDisabled_stillClaimedAndReported(t *testing.T) {
	haAgent := haagentmock.NewMockHaAgent().(haagentmock.Component)
	haAgent.SetEnabled(true)

	agentConfigs := map[string]interface{}{
		"hostname":                         "my-agent-hostname",
		"agent_workload_balancing.enabled": false,
	}
	provides := newTestWorkloadBalancingComponent(t, agentConfigs, haAgent, nil)
	w := provides.Comp.(*workloadBalancingImpl)

	var applyID string
	var applyStatus state.ApplyStatus
	w.onWorkloadBalancingUpdate(map[string]state.RawConfig{
		testRCConfigPath: {Config: []byte(`{"type":"workload_balancing","workload_balancing_group_id":"group-01","active_agent":"another-agent-hostname"}`)},
	}, func(id string, status state.ApplyStatus) {
		applyID = id
		applyStatus = status
	})

	// Still claimed and reported.
	assert.Equal(t, testRCConfigPath, applyID)
	assert.Equal(t, state.ApplyStatus{State: state.ApplyStateAcknowledged}, applyStatus)
	w.mu.RLock()
	assert.Equal(t, groupStateStandby, w.groups["group-01"])
	w.mu.RUnlock()

	// But execution is unaffected: the local flag overrides the standby assignment.
	assert.True(t, w.IsGroupActive("group-01"))
}

func Test_onWorkloadBalancingUpdate_clearsGroupNoLongerPresent(t *testing.T) {
	w := newTestWorkloadBalancingImplWithHaAgent(t, true)

	// Batch 1: group-01 is assigned to another agent, so this agent is on standby.
	w.onWorkloadBalancingUpdate(map[string]state.RawConfig{
		testRCConfigPath: {Config: []byte(`{"type":"workload_balancing","workload_balancing_group_id":"group-01","active_agent":"another-agent-hostname"}`)},
	}, func(string, state.ApplyStatus) {})
	assert.False(t, w.IsGroupActive("group-01"))

	// Batch 2: a full, non-empty snapshot that no longer mentions group-01 at all -- its
	// assignment is gone, not just momentarily absent, so it reverts to unmanaged and runs.
	w.onWorkloadBalancingUpdate(map[string]state.RawConfig{
		"datadog/2/HA_AGENT/config-workload-balancing/2": {Config: []byte(`{"type":"workload_balancing","workload_balancing_group_id":"group-02","active_agent":"another-agent-hostname"}`)},
	}, func(string, state.ApplyStatus) {})

	assert.True(t, w.IsGroupActive("group-01"))
	assert.False(t, w.IsGroupActive("group-02"))
}
