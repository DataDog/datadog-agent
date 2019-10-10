// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build clusterchecks

package clusterchecks

import (
	"fmt"
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
		return http.StatusFound, fmt.Sprintf("%s:%d", h.leaderIP, h.port)
	default:
		return http.StatusServiceUnavailable, notReadyReason
	}
}

// GetState returns the state of the dispatching, for the clusterchecks cmd
func (h *Handler) GetState() (types.StateResponse, error) {
	h.m.RLock()
	defer h.m.RUnlock()

	switch h.state {
	case leader:
		return h.dispatcher.getState()
	case follower:
		return types.StateResponse{NotRunning: "currently follower"}, nil
	default:
		return types.StateResponse{NotRunning: notReadyReason}, nil
	}
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
func (h *Handler) PostStatus(nodeName, clientIP string, status types.NodeStatus) (types.StatusResponse, error) {
	upToDate, err := h.dispatcher.processNodeStatus(nodeName, clientIP, status)
	response := types.StatusResponse{
		IsUpToDate: upToDate,
	}
	return response, err
}

// GetEndpointsConfigs returns endpoints configurations dispatched to a given node
func (h *Handler) GetEndpointsConfigs(nodeName string) (types.ConfigResponse, error) {
	configs, err := h.dispatcher.getEndpointsConfigs(nodeName)
	response := types.ConfigResponse{
		Configs:    configs,
		LastChange: 0,
	}
	return response, err
}

// GetAllEndpointsCheckConfigs returns all pod-backed dispatched endpointscheck configurations
func (h *Handler) GetAllEndpointsCheckConfigs() (types.ConfigResponse, error) {
	configs, err := h.dispatcher.getAllEndpointsCheckConfigs()
	response := types.ConfigResponse{
		Configs:    configs,
		LastChange: 0,
	}
	return response, err
}
