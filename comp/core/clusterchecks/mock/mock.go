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
)

// Handler implements clusterchecks.Component for testing
type Handler struct {
	State clusterchecks.StateResponse
	Err   error
}

// New creates a new mock handler with the given state and error
func New(state clusterchecks.StateResponse, err error) clusterchecks.Component {
	return &Handler{
		State: state,
		Err:   err,
	}
}

// NewDefault creates a new mock handler with default values
func NewDefault() clusterchecks.Component {
	return &Handler{
		State: clusterchecks.StateResponse{},
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
func (m *Handler) GetState() (clusterchecks.StateResponse, error) {
	return m.State, m.Err
}

// GetConfigs returns an empty response
func (m *Handler) GetConfigs(_ string) (clusterchecks.ConfigResponse, error) {
	return clusterchecks.ConfigResponse{}, nil
}

// PostStatus returns an empty response
func (m *Handler) PostStatus(_, _ string, _ clusterchecks.NodeStatus) clusterchecks.StatusResponse {
	return clusterchecks.StatusResponse{}
}

// GetEndpointsConfigs returns an empty response
func (m *Handler) GetEndpointsConfigs(_ string) (clusterchecks.ConfigResponse, error) {
	return clusterchecks.ConfigResponse{}, nil
}

// GetAllEndpointsCheckConfigs returns an empty response
func (m *Handler) GetAllEndpointsCheckConfigs() (clusterchecks.ConfigResponse, error) {
	return clusterchecks.ConfigResponse{}, nil
}

// RebalanceClusterChecks returns nil
func (m *Handler) RebalanceClusterChecks(_ bool) ([]clusterchecks.RebalanceResponse, error) {
	return nil, nil
}

// IsolateCheck returns an empty response
func (m *Handler) IsolateCheck(_ string) clusterchecks.IsolateResponse {
	return clusterchecks.IsolateResponse{}
}

// GetStats returns the configured state and error
func (m *Handler) GetStats() (*clusterchecks.Stats, error) {
	return &clusterchecks.Stats{}, m.Err
}

// GetNodeTypeCounts returns zero counts
func (m *Handler) GetNodeTypeCounts() (clcRunnerCount, nodeAgentCount int, err error) {
	return 0, 0, m.Err
}
