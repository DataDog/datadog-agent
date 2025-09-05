// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clustercheckimpl

import (
	"errors"

	clusterchecks "github.com/DataDog/datadog-agent/comp/core/clusterchecks/def"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
)

const handlerCacheKey = "cluster_checks_handler"

// GetStats retrieves the stats of the latest started handler.
// It is used for the agent status command.
func GetStats() (*clusterchecks.Stats, error) {
	key := cache.BuildAgentKey(handlerCacheKey)
	x, found := cache.Cache.Get(key)
	if !found {
		return nil, errors.New("Clusterchecks not running")
	}

	handler, ok := x.(*Handler)
	if !ok {
		return nil, errors.New("Cache entry is not a valid handler")
	}

	return handler.getStats(), nil
}

// GetNodeTypeCounts returns the number of CLC runners and node agents running cluster checks.
func GetNodeTypeCounts() (clcRunnerCount, nodeAgentCount int, err error) {
	key := cache.BuildAgentKey(handlerCacheKey)
	x, found := cache.Cache.Get(key)
	if !found {
		return 0, 0, errors.New("Clusterchecks not running")
	}

	handler, ok := x.(*Handler)
	if !ok {
		return 0, 0, errors.New("Cache entry is not a valid handler")
	}

	clcRunnerCount, nodeAgentCount = handler.dispatcher.store.CountNodeTypes()
	return clcRunnerCount, nodeAgentCount, nil
}

// GetNodeTypeCounts returns the number of CLC runners and node agents (method on Handler)
func (h *Handler) GetNodeTypeCounts() (clcRunnerCount, nodeAgentCount int, err error) {
	clcRunnerCount, nodeAgentCount = h.dispatcher.store.CountNodeTypes()
	return clcRunnerCount, nodeAgentCount, nil
}

// GetStats returns the stats of the handler
func (h *Handler) GetStats() (*clusterchecks.Stats, error) {
	return h.getStats(), nil
}

func (h *Handler) getStats() *clusterchecks.Stats {
	h.m.RLock()
	defer h.m.RUnlock()

	switch h.state {
	case leader:
		s := h.dispatcher.getStats()
		s.Leader = true
		return s
	case follower:
		return &clusterchecks.Stats{
			Follower: true,
			LeaderIP: h.leaderIP,
		}
	default:
		// Unknown state, leave both Leader & Follower false
		return &clusterchecks.Stats{}
	}
}

func (d *dispatcher) getStats() *clusterchecks.Stats {
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
	return &clusterchecks.Stats{
		Active:            d.store.active,
		NodeCount:         len(d.store.nodes),
		ActiveConfigs:     len(d.store.digestToNode),
		DanglingConfigs:   len(d.store.danglingConfigs),
		UnscheduledChecks: unscheduledChecks,
		TotalConfigs:      len(d.store.digestToConfig),
		CheckNames:        checkNames,
	}
}
