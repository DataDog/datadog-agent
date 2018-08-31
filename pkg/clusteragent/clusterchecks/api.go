// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build clusterchecks

package clusterchecks

import (
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
)

// ShouldRedirect returns the leader's hostname if the cluster-agent
// is currently a follower, or an empty string if we should handle the query
func (h *Handler) ShouldRedirect() string {
	return ""
}

// GetAllConfigs returns all configurations known to the store, for reporting
func (h *Handler) GetAllConfigs() (types.ConfigResponse, error) {
	h.store.RLock()
	defer h.store.RUnlock()
	response := types.ConfigResponse{
		Configs: h.store.getAllConfigs(),
	}
	return response, nil
}

// GetConfigs returns  configurations dispatched to a given node
func (h *Handler) GetConfigs(nodeName string) (types.ConfigResponse, error) {
	h.store.RLock()
	defer h.store.RUnlock()
	response := types.ConfigResponse{
		Configs: h.store.getNodeConfigs(nodeName),
	}
	return response, nil
}

// GetConfigs returns configurations dispatched to a given node
func (h *Handler) PostStatus(nodeName string, status types.NodeStatus) (types.StatusResponse, error) {
	h.store.Lock()
	defer h.store.Unlock()
	h.store.storeNodeStatus(nodeName, status)
	lastChange := h.store.getNodeLastChange(nodeName)

	response := types.StatusResponse{
		IsUpToDate: (lastChange == status.LastChange),
	}
	return response, nil
}
