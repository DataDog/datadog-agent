// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
)

const handlerCacheKey = "cluster_checks_handler"

// GetStats retrieves the stats of the latest started handler.
// It is used for the agent status command.
func GetStats() (*types.Stats, error) {
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
	return &types.Stats{
		Active:          d.store.active,
		NodeCount:       len(d.store.nodes),
		ActiveConfigs:   len(d.store.digestToNode),
		DanglingConfigs: len(d.store.danglingConfigs),
		TotalConfigs:    len(d.store.digestToConfig),
		CheckNames:      checkNames,
	}
}
