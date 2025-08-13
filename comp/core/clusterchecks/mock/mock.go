// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package mock provides a mock implementation of the clusterchecks component for testing
package mock

import (
	"context"
	"net/http"

	clusterchecks "github.com/DataDog/datadog-agent/comp/core/clusterchecks/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
)

// Handler implements clusterchecks.Component for testing
type Handler struct {
	State types.StateResponse
	Err   error
}

// New creates a new mock handler with the given state and error
func New(state types.StateResponse, err error) clusterchecks.Component {
	return &Handler{
		State: state,
		Err:   err,
	}
}

// NewDefault creates a new mock handler with default values
func NewDefault() clusterchecks.Component {
	return &Handler{
		State: types.StateResponse{},
		Err:   nil,
	}
}

// Run is a no-op for the mock
func (m *Handler) Run(_ context.Context) {}

// RejectOrForwardLeaderQuery always returns false for the mock
func (m *Handler) RejectOrForwardLeaderQuery(_ http.ResponseWriter, _ *http.Request) bool {
	return false
}

// GetState returns the configured state and error
func (m *Handler) GetState() (types.StateResponse, error) {
	return m.State, m.Err
}

// GetConfigs returns an empty response
func (m *Handler) GetConfigs(_ string) (types.ConfigResponse, error) {
	return types.ConfigResponse{}, nil
}

// PostStatus returns an empty response
func (m *Handler) PostStatus(_, _ string, _ types.NodeStatus) types.StatusResponse {
	return types.StatusResponse{}
}

// GetEndpointsConfigs returns an empty response
func (m *Handler) GetEndpointsConfigs(_ string) (types.ConfigResponse, error) {
	return types.ConfigResponse{}, nil
}

// GetAllEndpointsCheckConfigs returns an empty response
func (m *Handler) GetAllEndpointsCheckConfigs() (types.ConfigResponse, error) {
	return types.ConfigResponse{}, nil
}

// RebalanceClusterChecks returns nil
func (m *Handler) RebalanceClusterChecks(_ bool) ([]types.RebalanceResponse, error) {
	return nil, nil
}

// IsolateCheck returns an empty response
func (m *Handler) IsolateCheck(_ string) types.IsolateResponse {
	return types.IsolateResponse{}
}

// GetStats returns the configured state and error
func (m *Handler) GetStats() (*types.Stats, error) {
	return &types.Stats{}, m.Err
}

// GetNodeTypeCounts returns zero counts
func (m *Handler) GetNodeTypeCounts() (clcRunnerCount, nodeAgentCount int, err error) {
	return 0, 0, m.Err
}
