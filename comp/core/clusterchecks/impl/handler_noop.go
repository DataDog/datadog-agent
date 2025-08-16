// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !clusterchecks

// Package clusterchecksimpl contains a no-op implementation of the clusterchecks handler
package clusterchecksimpl

import (
	"context"
	"errors"
	"net/http"

	clusterchecks "github.com/DataDog/datadog-agent/comp/core/clusterchecks/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
)

// noopImpl is a no-op implementation of the clusterchecks handler
type noopImpl struct{}

// Requires defines the dependencies for the no-op clusterchecks handler
type Requires struct {
	compdef.In
}

// Provides defines the output of the no-op clusterchecks handler
type Provides struct {
	compdef.Out

	Component clusterchecks.Component
}

// NewComponent creates a new no-op clusterchecks handler component
func NewComponent(_ Requires) (Provides, error) {
	return Provides{
		Component: &noopImpl{},
	}, nil
}

// Run is a no-op
func (n *noopImpl) Run(_ context.Context) {}

// RejectOrForwardLeaderQuery always returns false for no-op
func (n *noopImpl) RejectOrForwardLeaderQuery(_ http.ResponseWriter, _ *http.Request) bool {
	return false
}

// GetState returns an error for no-op
func (n *noopImpl) GetState() (types.StateResponse, error) {
	return types.StateResponse{}, errors.New("cluster checks not compiled")
}

// GetConfigs returns an error for no-op
func (n *noopImpl) GetConfigs(_ string) (types.ConfigResponse, error) {
	return types.ConfigResponse{}, errors.New("cluster checks not compiled")
}

// PostStatus returns an error response for no-op
func (n *noopImpl) PostStatus(_, _ string, _ types.NodeStatus) types.StatusResponse {
	return types.StatusResponse{IsUpToDate: false}
}

// GetEndpointsConfigs returns an error for no-op
func (n *noopImpl) GetEndpointsConfigs(_ string) (types.ConfigResponse, error) {
	return types.ConfigResponse{}, errors.New("cluster checks not compiled")
}

// GetAllEndpointsCheckConfigs returns an error for no-op
func (n *noopImpl) GetAllEndpointsCheckConfigs() (types.ConfigResponse, error) {
	return types.ConfigResponse{}, errors.New("cluster checks not compiled")
}

// RebalanceClusterChecks returns an error for no-op
func (n *noopImpl) RebalanceClusterChecks(_ bool) ([]types.RebalanceResponse, error) {
	return nil, errors.New("cluster checks not compiled")
}

// IsolateCheck returns an error response for no-op
func (n *noopImpl) IsolateCheck(_ string) types.IsolateResponse {
	return types.IsolateResponse{Reason: "cluster checks not compiled"}
}

// GetStats returns an error for no-op
func (n *noopImpl) GetStats() (*types.Stats, error) {
	return nil, errors.New("cluster checks not compiled")
}

// GetNodeTypeCounts returns an error for no-op
func (n *noopImpl) GetNodeTypeCounts() (clcRunnerCount, nodeAgentCount int, err error) {
	return 0, 0, errors.New("cluster checks not compiled")
}
