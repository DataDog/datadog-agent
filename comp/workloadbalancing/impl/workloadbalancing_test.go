// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package workloadbalancingimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"

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

func TestGroupStateTransitions(t *testing.T) {
	comp := newTestWorkloadBalancingComponent(t, map[string]interface{}{
		"hostname":                         testHostname,
		"agent_workload_balancing.enabled": true,
	}, nil).Comp

	// a group we were never told about is unmanaged, and runs
	assert.Equal(t, workloadbalancing.Unmanaged, comp.GetGroupState("group1"))
	assert.True(t, comp.IsGroupActive("group1"))

	// another Agent holds the group, so we stand by
	comp.SetGroupLeader("group1", "another-agent-hostname")
	assert.Equal(t, workloadbalancing.Standby, comp.GetGroupState("group1"))
	assert.False(t, comp.IsGroupActive("group1"))

	// we hold the group
	comp.SetGroupLeader("group1", testHostname)
	assert.Equal(t, workloadbalancing.Active, comp.GetGroupState("group1"))
	assert.True(t, comp.IsGroupActive("group1"))

	// dropping the assignment returns the group to unmanaged, which runs
	comp.RemoveGroup("group1")
	assert.Equal(t, workloadbalancing.Unmanaged, comp.GetGroupState("group1"))
	assert.True(t, comp.IsGroupActive("group1"))
}

func TestGroupsAreIndependent(t *testing.T) {
	comp := newTestWorkloadBalancingComponent(t, map[string]interface{}{
		"hostname":                         testHostname,
		"agent_workload_balancing.enabled": true,
	}, nil).Comp

	comp.SetGroupLeader("group1", testHostname)
	comp.SetGroupLeader("group2", "another-agent-hostname")

	assert.True(t, comp.IsGroupActive("group1"))
	assert.False(t, comp.IsGroupActive("group2"))

	comp.RemoveGroup("group1")

	assert.True(t, comp.IsGroupActive("group1"))
	assert.Equal(t, workloadbalancing.Standby, comp.GetGroupState("group2"))
}

func TestRemoveUnknownGroup(t *testing.T) {
	comp := newTestWorkloadBalancingComponent(t, map[string]interface{}{
		"hostname": testHostname,
	}, nil).Comp

	comp.RemoveGroup("never-seen")
	assert.Empty(t, comp.GetGroupStates())
}

func TestGetGroupStates(t *testing.T) {
	comp := newTestWorkloadBalancingComponent(t, map[string]interface{}{
		"hostname": testHostname,
	}, nil).Comp

	assert.Empty(t, comp.GetGroupStates())

	comp.SetGroupLeader("group1", testHostname)
	comp.SetGroupLeader("group2", "another-agent-hostname")

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
