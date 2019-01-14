// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build clusterchecks

package clusterchecks

// Status holds information for the status and flare commands
type Status struct {
	// Following
	Follower bool
	LeaderIP string

	// Leading
	Leader          bool
	Active          bool
	NodeCount       int
	ActiveConfigs   int
	DanglingConfigs int
	TotalConfigs    int
}

// GetAgentStatus collects information for the status and flare commands
func (h *Handler) GetAgentStatus() (string, interface{}) {
	h.m.RLock()
	defer h.m.RUnlock()

	var s *Status

	switch h.state {
	case leader:
		s = h.dispatcher.getAgentStatus()
		s.Leader = true
	case follower:
		s = &Status{
			Follower: true,
			LeaderIP: h.leaderIP,
		}
	default:
		// Unknown state, leave both Leader & Follower false
		s = &Status{}
	}
	return "clusterchecks", s
}

func (d *dispatcher) getAgentStatus() *Status {
	d.store.RLock()
	defer d.store.RUnlock()

	return &Status{
		Active:          d.store.active,
		NodeCount:       len(d.store.nodes),
		ActiveConfigs:   len(d.store.digestToNode),
		DanglingConfigs: len(d.store.danglingConfigs),
		TotalConfigs:    len(d.store.digestToConfig),
	}
}
