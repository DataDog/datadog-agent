// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package workloadbalancingimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"

	haagentmock "github.com/DataDog/datadog-agent/comp/haagent/mock"
)

func Test_RCListener_alwaysRegisters(t *testing.T) {
	tests := []struct {
		name    string
		configs map[string]interface{}
	}{
		{name: "enabled", configs: map[string]interface{}{"agent_workload_balancing.enabled": true}},
		{name: "disabled", configs: map[string]interface{}{"agent_workload_balancing.enabled": false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provides := newTestWorkloadBalancingComponent(t, tt.configs, nil, nil)
			assert.NotNil(t, provides.RCListener.ListenerProvider)
		})
	}
}

func Test_IsGroupActive(t *testing.T) {
	tests := []struct {
		name         string
		enabled      bool
		groups       map[string]groupState
		groupID      string
		expectActive bool
	}{
		{
			name:         "flag disabled, group on standby, still active",
			enabled:      false,
			groups:       map[string]groupState{"group-01": groupStateStandby},
			groupID:      "group-01",
			expectActive: true,
		},
		{
			name:         "flag enabled, group on standby, not active",
			enabled:      true,
			groups:       map[string]groupState{"group-01": groupStateStandby},
			groupID:      "group-01",
			expectActive: false,
		},
		{
			name:         "flag enabled, group active",
			enabled:      true,
			groups:       map[string]groupState{"group-01": groupStateActive},
			groupID:      "group-01",
			expectActive: true,
		},
		{
			name:         "flag enabled, group never seen (map-miss), active",
			enabled:      true,
			groups:       map[string]groupState{},
			groupID:      "group-01",
			expectActive: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agentConfigs := map[string]interface{}{"agent_workload_balancing.enabled": tt.enabled}
			provides := newTestWorkloadBalancingComponent(t, agentConfigs, haagentmock.NewMockHaAgent(), nil)
			w := provides.Comp.(*workloadBalancingImpl)
			w.groups = tt.groups

			assert.Equal(t, tt.expectActive, w.IsGroupActive(tt.groupID))
		})
	}
}
