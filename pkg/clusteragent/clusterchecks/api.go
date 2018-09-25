// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build clusterchecks

package clusterchecks

import (
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
)

// TODO: handle non-leader / warmup phases, handler methods will
// become more complex at that stage

// ShouldRedirect returns the leader's hostname if the cluster-agent
// is currently a follower, or an empty string if we should handle the query
func (h *Handler) ShouldRedirect() string {
	return ""
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
