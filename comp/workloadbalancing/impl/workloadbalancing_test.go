// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package workloadbalancingimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workloadbalancing "github.com/DataDog/datadog-agent/comp/workloadbalancing/def"
)

const testHostname = "my-agent-hostname"

func TestEnabled(t *testing.T) {
	tests := []struct {
		name            string
		configs         map[string]interface{}
		expectedEnabled bool
	}{
		{
			name:            "not set",
			configs:         map[string]interface{}{},
			expectedEnabled: false,
		},
		{
			name: "explicitly disabled",
			configs: map[string]interface{}{
				"agent_workload_balancing.enabled": false,
			},
			expectedEnabled: false,
		},
		{
			name: "enabled",
			configs: map[string]interface{}{
				"agent_workload_balancing.enabled": true,
			},
			expectedEnabled: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comp := newTestWorkloadBalancingComponent(t, tt.configs, nil).Comp
			assert.Equal(t, tt.expectedEnabled, comp.Enabled())
		})
	}
}

func newStateTestComponent(t *testing.T) *workloadBalancingImpl {
	t.Helper()
	provides := newTestWorkloadBalancingComponent(t, map[string]interface{}{
		"hostname":                         testHostname,
		"agent_workload_balancing.enabled": true,
	}, nil)
	comp, ok := provides.Comp.(*workloadBalancingImpl)
	require.True(t, ok)
	return comp
}

func TestGroupStateTransitions(t *testing.T) {
	comp := newStateTestComponent(t)

	// a group we were never told about is unmanaged, and runs
	assert.Equal(t, workloadbalancing.Unmanaged, comp.GetGroupState("group1"))
	assert.True(t, comp.IsGroupActive("group1"))

	// another Agent holds the group, so we stand by
	require.NoError(t, comp.setGroupLeaders(map[string]string{"group1": "another-agent-hostname"}))
	assert.Equal(t, workloadbalancing.Standby, comp.GetGroupState("group1"))
	assert.False(t, comp.IsGroupActive("group1"))

	// we hold the group
	require.NoError(t, comp.setGroupLeaders(map[string]string{"group1": testHostname}))
	assert.Equal(t, workloadbalancing.Active, comp.GetGroupState("group1"))
	assert.True(t, comp.IsGroupActive("group1"))

	// dropping the assignment returns the group to unmanaged, which runs
	require.NoError(t, comp.setGroupLeaders(map[string]string{}))
	assert.Equal(t, workloadbalancing.Unmanaged, comp.GetGroupState("group1"))
	assert.True(t, comp.IsGroupActive("group1"))
}

// A group nobody has been assigned must keep running, or the device it covers goes unpolled.
func TestGroupWithNoAssignedAgentRuns(t *testing.T) {
	comp := newStateTestComponent(t)

	require.NoError(t, comp.setGroupLeaders(map[string]string{"group1": ""}))

	assert.Equal(t, workloadbalancing.Unmanaged, comp.GetGroupState("group1"))
	assert.True(t, comp.IsGroupActive("group1"))

	// the group is still reported, so a group with no active Agent anywhere is visible
	assert.Equal(t, map[string]workloadbalancing.State{
		"group1": workloadbalancing.Unmanaged,
	}, comp.GetGroupStates())
}

func TestGroupsAreIndependent(t *testing.T) {
	comp := newStateTestComponent(t)

	require.NoError(t, comp.setGroupLeaders(map[string]string{
		"group1": testHostname,
		"group2": "another-agent-hostname",
	}))

	assert.True(t, comp.IsGroupActive("group1"))
	assert.False(t, comp.IsGroupActive("group2"))

	require.NoError(t, comp.setGroupLeaders(map[string]string{
		"group2": "another-agent-hostname",
	}))

	assert.True(t, comp.IsGroupActive("group1"))
	assert.Equal(t, workloadbalancing.Standby, comp.GetGroupState("group2"))
}

// A hostname we cannot resolve leaves every group exactly as it was.
func TestHostnameFailureLeavesStateUnchanged(t *testing.T) {
	comp := newStateTestComponent(t)

	require.NoError(t, comp.setGroupLeaders(map[string]string{"group1": "another-agent-hostname"}))
	require.Equal(t, workloadbalancing.Standby, comp.GetGroupState("group1"))

	comp.hostname = failingHostname{}
	err := comp.setGroupLeaders(map[string]string{"group1": testHostname})

	assert.Error(t, err)
	assert.Equal(t, workloadbalancing.Standby, comp.GetGroupState("group1"))
}

func TestGetGroupStates(t *testing.T) {
	comp := newStateTestComponent(t)

	assert.Empty(t, comp.GetGroupStates())

	require.NoError(t, comp.setGroupLeaders(map[string]string{
		"group1": testHostname,
		"group2": "another-agent-hostname",
	}))

	assert.Equal(t, map[string]workloadbalancing.State{
		"group1": workloadbalancing.Active,
		"group2": workloadbalancing.Standby,
	}, comp.GetGroupStates())

	// the returned map is a copy, so a caller cannot mutate our state
	states := comp.GetGroupStates()
	states["group1"] = workloadbalancing.Standby
	delete(states, "group2")

	assert.Equal(t, workloadbalancing.Active, comp.GetGroupState("group1"))
	assert.Equal(t, workloadbalancing.Standby, comp.GetGroupState("group2"))
}
