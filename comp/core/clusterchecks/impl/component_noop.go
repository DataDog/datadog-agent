// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !clusterchecks

package clustercheckimpl

import (
	"context"
	"net/http"

	"go.uber.org/fx"

	clusterchecks "github.com/DataDog/datadog-agent/comp/core/clusterchecks/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
)

type dependencies struct {
	fx.In

	Config config.Component
}

type provides struct {
	fx.Out

	Component clusterchecks.Component
}

// NewComponent creates a new clusterchecks component (noop when build tag is not set)
func NewComponent(deps dependencies) provides {
	return provides{
		Component: &noopImpl{},
	}
}

// noopImpl is a no-op implementation of the clusterchecks component
type noopImpl struct{}

// Run is a no-op
func (n *noopImpl) Run(ctx context.Context) {}

// RejectOrForwardLeaderQuery always returns false for noop
func (n *noopImpl) RejectOrForwardLeaderQuery(rw http.ResponseWriter, req *http.Request) bool {
	return false
}

// GetState returns empty state for noop
func (n *noopImpl) GetState() (clusterchecks.StateResponse, error) {
	return clusterchecks.StateResponse{NotRunning: "clusterchecks disabled"}, nil
}

// GetConfigs returns empty config for noop
func (n *noopImpl) GetConfigs(identifier string) (clusterchecks.ConfigResponse, error) {
	return clusterchecks.ConfigResponse{}, nil
}

// PostStatus returns empty response for noop
func (n *noopImpl) PostStatus(identifier, clientIP string, status clusterchecks.NodeStatus) clusterchecks.StatusResponse {
	return clusterchecks.StatusResponse{}
}

// GetEndpointsConfigs returns empty config for noop
func (n *noopImpl) GetEndpointsConfigs(nodeName string) (clusterchecks.ConfigResponse, error) {
	return clusterchecks.ConfigResponse{}, nil
}

// GetAllEndpointsCheckConfigs returns empty config for noop
func (n *noopImpl) GetAllEndpointsCheckConfigs() (clusterchecks.ConfigResponse, error) {
	return clusterchecks.ConfigResponse{}, nil
}

// RebalanceClusterChecks returns empty response for noop
func (n *noopImpl) RebalanceClusterChecks(force bool) ([]clusterchecks.RebalanceResponse, error) {
	return nil, nil
}

// IsolateCheck returns empty response for noop
func (n *noopImpl) IsolateCheck(isolateCheckID string) clusterchecks.IsolateResponse {
	return clusterchecks.IsolateResponse{}
}

// GetStats returns nil for noop
func (n *noopImpl) GetStats() (*clusterchecks.Stats, error) {
	return nil, nil
}

// GetNodeTypeCounts returns zeros for noop
func (n *noopImpl) GetNodeTypeCounts() (clcRunnerCount, nodeAgentCount int, err error) {
	return 0, 0, nil
}
