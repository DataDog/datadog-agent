// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
)

// GetStats retrieves the stats of the handler.
// It is used for the agent status command.
func (h *Handler) GetStats() (*types.Stats, error) {
	return h.getStats(), nil
}

// GetNodeTypeCounts returns the number of CLC runners and node agents running cluster checks.
func (h *Handler) GetNodeTypeCounts() (clcRunnerCount, nodeAgentCount int, err error) {
	clcRunnerCount, nodeAgentCount = h.dispatcher.store.CountNodeTypes()
	return clcRunnerCount, nodeAgentCount, nil
}

func (h *Handler) getStats() *types.Stats {
	h.m.RLock()
	defer h.m.RUnlock()

	switch h.state {
	case leader:
		s := h.dispatcher.getStats()
		s.Leader = true
		return s
	case follower:
		return &types.Stats{
			Follower: true,
			LeaderIP: h.leaderIP,
		}
	default:
		// Unknown state, leave both Leader & Follower false
		return &types.Stats{}
	}
}

func (d *dispatcher) getStats() *types.Stats {
	d.store.RLock()
	defer d.store.RUnlock()
	checkNames := make(map[string]struct{})
	for _, m := range d.store.digestToConfig {
		checkNames[m.Name] = struct{}{}
	}
	unscheduledChecks := 0
	for _, c := range d.store.danglingConfigs {
		if c.unscheduledCheck {
			unscheduledChecks++
		}
	}
	return &types.Stats{
		Active:            d.store.active,
		NodeCount:         len(d.store.nodes),
		ActiveConfigs:     len(d.store.digestToNode),
		DanglingConfigs:   len(d.store.danglingConfigs),
		UnscheduledChecks: unscheduledChecks,
		TotalConfigs:      len(d.store.digestToConfig),
		CheckNames:        checkNames,
	}
}
