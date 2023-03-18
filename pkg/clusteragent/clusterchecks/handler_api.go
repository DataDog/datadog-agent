// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
)

var errNotReady = errors.New("Startup in progress")

// RejectOrForwardLeaderQuery performs some checks on incoming queries that should go to a leader:
// - Forward to leader if we are a follower
// - Reject with "not ready" if leader election status is unknown
func (h *Handler) RejectOrForwardLeaderQuery(rw http.ResponseWriter, req *http.Request) bool {
	h.m.RLock()
	defer h.m.RUnlock()

	switch h.state {
	case leader:
		return false
	case follower:
		if h.leaderForwarder == nil {
			http.Error(rw, "Follower unable to forward as leaderForwarder is not available yet", http.StatusServiceUnavailable)
			return true
		}

		h.leaderForwarder.Forward(rw, req)
		return true
	case unknown:
		fallthrough
	default:
		http.Error(rw, errNotReady.Error(), http.StatusServiceUnavailable)
		return true
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
		return types.StateResponse{NotRunning: errNotReady.Error()}, nil
	}
}

// GetConfigs returns configurations dispatched to a given agent
func (h *Handler) GetConfigs(identifier string) (types.ConfigResponse, error) {
	configs, lastChange, err := h.dispatcher.getClusterCheckConfigs(identifier)
	response := types.ConfigResponse{
		Configs:    configs,
		LastChange: lastChange,
	}
	return response, err
}

// PostStatus handles status reports from the node agents
func (h *Handler) PostStatus(identifier, clientIP string, status types.NodeStatus) (types.StatusResponse, error) {
	upToDate, err := h.dispatcher.processNodeStatus(identifier, clientIP, status)
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

// RebalanceClusterChecks triggers an attempt to rebalance cluster checks
func (h *Handler) RebalanceClusterChecks() ([]types.RebalanceResponse, error) {
	if !h.dispatcher.advancedDispatching {
		return nil, fmt.Errorf("no checks to rebalance: advanced dispatching is not enabled")
	}

	rebalancingDecisions := h.dispatcher.rebalance()
	response := []types.RebalanceResponse{}

	for _, decision := range rebalancingDecisions {
		response = append(response, types.RebalanceResponse{
			CheckID:        decision.CheckID,
			CheckWeight:    decision.CheckWeight,
			SourceNodeName: decision.SourceNodeName,
			SourceDiff:     decision.SourceDiff,
			DestNodeName:   decision.DestNodeName,
			DestDiff:       decision.DestDiff,
		})
	}

	return response, nil
}
