// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package clusterchecks provides the cluster checks handler component
package clusterchecks

import (
	"context"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
)

// team: container-platform

// Component is the interface for the cluster checks handler
type Component interface {
	// Run is the main goroutine for the handler. It has to
	// be called in a goroutine with a cancellable context.
	Run(ctx context.Context)

	// API Methods

	// RejectOrForwardLeaderQuery rejects or forwards the query based on leadership status
	RejectOrForwardLeaderQuery(rw http.ResponseWriter, req *http.Request) bool

	// GetState returns the state of the cluster checks handler
	GetState() (types.StateResponse, error)

	// GetConfigs returns configurations for a specific identifier
	GetConfigs(identifier string) (types.ConfigResponse, error)

	// PostStatus updates the status for a specific identifier
	PostStatus(identifier, clientIP string, status types.NodeStatus) types.StatusResponse

	// GetEndpointsConfigs returns endpoints configurations for a specific node
	GetEndpointsConfigs(nodeName string) (types.ConfigResponse, error)

	// GetAllEndpointsCheckConfigs returns all endpoints check configurations
	GetAllEndpointsCheckConfigs() (types.ConfigResponse, error)

	// RebalanceClusterChecks triggers a rebalancing of cluster checks
	RebalanceClusterChecks(force bool) ([]types.RebalanceResponse, error)

	// IsolateCheck isolates a specific check
	IsolateCheck(isolateCheckID string) types.IsolateResponse

	// Stats Methods

	// GetStats retrieves the stats of the handler
	GetStats() (*types.Stats, error)

	// GetNodeTypeCounts returns the number of CLC runners and node agents running cluster checks
	GetNodeTypeCounts() (clcRunnerCount, nodeAgentCount int, err error)
}
