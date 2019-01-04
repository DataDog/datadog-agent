// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build clusterchecks

package clusterchecks

import (
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
)

const notReadyReason = "Startup in progress"

// ShouldHandle indicates whether the cluster-agent should serve cluster-check
// requests. Current known responses:
//   - 302, string: follower, leader IP in string
//   - 503, string: not ready, error string returned
//   - 200, "": leader and ready for serving requests
func (h *Handler) ShouldHandle() (int, string) {
	h.m.RLock()
	defer h.m.RUnlock()

	switch h.state {
	case leader:
		return http.StatusOK, ""
	case follower:
		return http.StatusFound, h.leaderIP
	default:
		return http.StatusServiceUnavailable, notReadyReason
	}
}

// GetAllConfigs returns all configurations known to the store, for reporting
func (h *Handler) GetAllConfigs() (types.ConfigResponse, error) {
	configs, err := h.dispatcher.getAllConfigs()
	response := types.ConfigResponse{
		Configs: configs,
	}
	return response, err
}

// GetConfigs returns configurations dispatched to a given node
func (h *Handler) GetConfigs(nodeName string) (types.ConfigResponse, error) {
	configs, lastChange, err := h.dispatcher.getNodeConfigs(nodeName)
	response := types.ConfigResponse{
		Configs:    configs,
		LastChange: lastChange,
	}
	return response, err
}

// PostStatus handles status reports from the node agents
func (h *Handler) PostStatus(nodeName string, status types.NodeStatus) (types.StatusResponse, error) {
	upToDate, err := h.dispatcher.processNodeStatus(nodeName, status)
	response := types.StatusResponse{
		IsUpToDate: upToDate,
	}
	return response, err
}
