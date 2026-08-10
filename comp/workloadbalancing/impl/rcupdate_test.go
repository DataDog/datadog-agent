// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package workloadbalancingimpl

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workloadbalancing "github.com/DataDog/datadog-agent/comp/workloadbalancing/def"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// rcUpdate builds the update map the RC client would hand us, one config per group.
func rcUpdate(groupToActiveAgent map[string]string) map[string]state.RawConfig {
	updates := make(map[string]state.RawConfig, len(groupToActiveAgent))
	for groupID, activeAgent := range groupToActiveAgent {
		path := fmt.Sprintf("datadog/2/NDM_AGENT_WORKLOAD_BALANCING/%s/config", groupID)
		updates[path] = state.RawConfig{
			Config: []byte(fmt.Sprintf(`{"group_id":%q,"active_agent":%q}`, groupID, activeAgent)),
		}
	}
	return updates
}

func collectApplyStates(t *testing.T) (func(string, state.ApplyStatus), map[string]state.ApplyStatus) {
	t.Helper()
	applied := map[string]state.ApplyStatus{}
	return func(path string, status state.ApplyStatus) {
		applied[path] = status
	}, applied
}

func newRCTestComponent(t *testing.T) *workloadBalancingImpl {
	t.Helper()
	provides := newTestWorkloadBalancingComponent(t, map[string]interface{}{
		"hostname":                         testHostname,
		"agent_workload_balancing.enabled": true,
	}, nil)
	comp, ok := provides.Comp.(*workloadBalancingImpl)
	require.True(t, ok)
	return comp
}

func TestRCListenerRegisteredOnlyWhenEnabled(t *testing.T) {
	disabled := newTestWorkloadBalancingComponent(t, map[string]interface{}{}, nil)
	assert.Nil(t, disabled.RCListener.ListenerProvider)

	enabled := newTestWorkloadBalancingComponent(t, map[string]interface{}{
		"agent_workload_balancing.enabled": true,
	}, nil)
	_, registered := enabled.RCListener.ListenerProvider[state.ProductNDMAgentWorkloadBalancing]
	assert.True(t, registered)
}

func TestOnUpdateAppliesAssignments(t *testing.T) {
	comp := newRCTestComponent(t)
	callback, applied := collectApplyStates(t)

	comp.onWorkloadBalancingUpdate(rcUpdate(map[string]string{
		"group1": testHostname,
		"group2": "another-agent-hostname",
	}), callback)

	assert.Equal(t, workloadbalancing.Active, comp.GetGroupState("group1"))
	assert.Equal(t, workloadbalancing.Standby, comp.GetGroupState("group2"))

	assert.Len(t, applied, 2)
	for path, status := range applied {
		assert.Equal(t, state.ApplyStateAcknowledged, status.State, path)
	}
}

// An empty update means no group targets this Agent any more. Every group returns to unmanaged and
// keeps reporting, which is the opposite of what the HA Agent does with an empty update.
func TestOnUpdateEmptyClearsAllGroups(t *testing.T) {
	comp := newRCTestComponent(t)
	callback, _ := collectApplyStates(t)

	comp.onWorkloadBalancingUpdate(rcUpdate(map[string]string{
		"group1": "another-agent-hostname",
	}), callback)
	require.Equal(t, workloadbalancing.Standby, comp.GetGroupState("group1"))

	comp.onWorkloadBalancingUpdate(map[string]state.RawConfig{}, callback)

	assert.Equal(t, workloadbalancing.Unmanaged, comp.GetGroupState("group1"))
	assert.True(t, comp.IsGroupActive("group1"))
	assert.Empty(t, comp.GetGroupStates())
}

// Remote Config resends the full set on every update, so a group that drops out of the set has lost
// its assignment and must not stay on standby.
func TestOnUpdateDropsGroupsMissingFromTheNewSet(t *testing.T) {
	comp := newRCTestComponent(t)
	callback, _ := collectApplyStates(t)

	comp.onWorkloadBalancingUpdate(rcUpdate(map[string]string{
		"group1": "another-agent-hostname",
		"group2": testHostname,
	}), callback)

	comp.onWorkloadBalancingUpdate(rcUpdate(map[string]string{
		"group2": testHostname,
	}), callback)

	assert.Equal(t, workloadbalancing.Unmanaged, comp.GetGroupState("group1"))
	assert.True(t, comp.IsGroupActive("group1"))
	assert.Equal(t, workloadbalancing.Active, comp.GetGroupState("group2"))
}

func TestOnUpdateInvalidPayloads(t *testing.T) {
	tests := []struct {
		name          string
		config        string
		expectedError string
	}{
		{
			name:          "malformed json",
			config:        `{"group_id":`,
			expectedError: "error unmarshalling payload",
		},
		{
			name:          "missing group id",
			config:        `{"active_agent":"some-agent"}`,
			expectedError: "group_id is empty",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comp := newRCTestComponent(t)
			callback, applied := collectApplyStates(t)

			const path = "datadog/2/NDM_AGENT_WORKLOAD_BALANCING/bad/config"
			comp.onWorkloadBalancingUpdate(map[string]state.RawConfig{
				path: {Config: []byte(tt.config)},
			}, callback)

			require.Contains(t, applied, path)
			assert.Equal(t, state.ApplyStateError, applied[path].State)
			assert.Equal(t, tt.expectedError, applied[path].Error)
			assert.Empty(t, comp.GetGroupStates())
		})
	}
}

// A bad config must not take the good ones down with it.
func TestOnUpdateAppliesValidConfigsAlongsideInvalidOnes(t *testing.T) {
	comp := newRCTestComponent(t)
	callback, applied := collectApplyStates(t)

	const goodPath = "datadog/2/NDM_AGENT_WORKLOAD_BALANCING/group1/config"
	const badPath = "datadog/2/NDM_AGENT_WORKLOAD_BALANCING/bad/config"
	comp.onWorkloadBalancingUpdate(map[string]state.RawConfig{
		goodPath: {Config: []byte(fmt.Sprintf(`{"group_id":"group1","active_agent":%q}`, testHostname))},
		badPath:  {Config: []byte(`not json`)},
	}, callback)

	assert.Equal(t, workloadbalancing.Active, comp.GetGroupState("group1"))
	assert.Equal(t, state.ApplyStateAcknowledged, applied[goodPath].State)
	assert.Equal(t, state.ApplyStateError, applied[badPath].State)
}

// A config that stops parsing must not silently hand its group back to this Agent.
func TestOnUpdateKeepsTheLastAssignmentForAnInvalidConfig(t *testing.T) {
	comp := newRCTestComponent(t)
	callback, applied := collectApplyStates(t)

	const path = "datadog/2/NDM_AGENT_WORKLOAD_BALANCING/group1/config"
	comp.onWorkloadBalancingUpdate(map[string]state.RawConfig{
		path: {Config: []byte(`{"group_id":"group1","active_agent":"another-agent"}`)},
	}, callback)
	require.Equal(t, workloadbalancing.Standby, comp.GetGroupState("group1"))

	comp.onWorkloadBalancingUpdate(map[string]state.RawConfig{
		path: {Config: []byte(`not json`)},
	}, callback)

	assert.Equal(t, workloadbalancing.Standby, comp.GetGroupState("group1"))
	assert.False(t, comp.IsGroupActive("group1"))
	assert.Equal(t, state.ApplyStateError, applied[path].State)

	// the carried-forward assignment survives a second bad update
	comp.onWorkloadBalancingUpdate(map[string]state.RawConfig{
		path: {Config: []byte(`{"active_agent":"another-agent"}`)},
	}, callback)
	assert.Equal(t, workloadbalancing.Standby, comp.GetGroupState("group1"))
}

// A config we can parse always beats a stale fallback for the same group, whichever order the
// update happens to iterate in.
func TestOnUpdateStaleFallbackDoesNotOverrideAFreshAssignment(t *testing.T) {
	comp := newRCTestComponent(t)
	callback, _ := collectApplyStates(t)

	const oldPath = "datadog/2/NDM_AGENT_WORKLOAD_BALANCING/group1-old/config"
	const newPath = "datadog/2/NDM_AGENT_WORKLOAD_BALANCING/group1-new/config"

	comp.onWorkloadBalancingUpdate(map[string]state.RawConfig{
		oldPath: {Config: []byte(`{"group_id":"group1","active_agent":"another-agent"}`)},
	}, callback)
	require.Equal(t, workloadbalancing.Standby, comp.GetGroupState("group1"))

	// group1 is handed to us on a new path while the old path stops parsing. Map iteration is
	// unordered, so run this enough times to hit both orders.
	for i := 0; i < 50; i++ {
		comp.onWorkloadBalancingUpdate(map[string]state.RawConfig{
			oldPath: {Config: []byte(`not json`)},
			newPath: {Config: []byte(fmt.Sprintf(`{"group_id":"group1","active_agent":%q}`, testHostname))},
		}, callback)
		require.Equal(t, workloadbalancing.Active, comp.GetGroupState("group1"))
	}
}

// A group the backend has not assigned to anyone keeps running here.
func TestOnUpdateEmptyActiveAgentLeavesTheGroupRunning(t *testing.T) {
	comp := newRCTestComponent(t)
	callback, applied := collectApplyStates(t)

	const path = "datadog/2/NDM_AGENT_WORKLOAD_BALANCING/group1/config"
	comp.onWorkloadBalancingUpdate(map[string]state.RawConfig{
		path: {Config: []byte(`{"group_id":"group1","active_agent":""}`)},
	}, callback)

	assert.Equal(t, workloadbalancing.Unmanaged, comp.GetGroupState("group1"))
	assert.True(t, comp.IsGroupActive("group1"))
	assert.Equal(t, state.ApplyStateAcknowledged, applied[path].State)
}

// A group ID reaches a metric tag unmodified, so an absurd one is rejected like an empty one.
func TestOnUpdateRejectsAnOverlongGroupID(t *testing.T) {
	comp := newRCTestComponent(t)
	callback, applied := collectApplyStates(t)

	const path = "datadog/2/NDM_AGENT_WORKLOAD_BALANCING/huge/config"
	comp.onWorkloadBalancingUpdate(map[string]state.RawConfig{
		path: {Config: []byte(fmt.Sprintf(`{"group_id":%q,"active_agent":"another-agent"}`,
			strings.Repeat("g", maxGroupIDLength+1)))},
	}, callback)

	assert.Equal(t, state.ApplyStateError, applied[path].State)
	assert.Empty(t, comp.GetGroupStates())
}

// A config that disappears from the update is gone for good, invalid or not.
func TestOnUpdateDropsAConfigThatIsNoLongerSent(t *testing.T) {
	comp := newRCTestComponent(t)
	callback, _ := collectApplyStates(t)

	const path = "datadog/2/NDM_AGENT_WORKLOAD_BALANCING/group1/config"
	comp.onWorkloadBalancingUpdate(map[string]state.RawConfig{
		path: {Config: []byte(`{"group_id":"group1","active_agent":"another-agent"}`)},
	}, callback)
	require.Equal(t, workloadbalancing.Standby, comp.GetGroupState("group1"))

	comp.onWorkloadBalancingUpdate(map[string]state.RawConfig{}, callback)

	assert.Equal(t, workloadbalancing.Unmanaged, comp.GetGroupState("group1"))
	assert.True(t, comp.IsGroupActive("group1"))
}
