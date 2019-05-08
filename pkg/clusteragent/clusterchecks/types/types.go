// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package types

import "github.com/StackVista/stackstate-agent/pkg/autodiscovery/integration"

// NodeStatus holds the status report from the node-agent
type NodeStatus struct {
	LastChange int64 `json:"last_change"`
}

// StatusResponse holds the DCA response for a status report
type StatusResponse struct {
	IsUpToDate bool `json:"isuptodate"`
}

// ConfigResponse holds the DCA response for a config query
type ConfigResponse struct {
	LastChange int64                `json:"last_change"`
	Configs    []integration.Config `json:"configs"`
}

// StateResponse holds the DCA response for a dispatching state query
type StateResponse struct {
	NotRunning string               `json:"not_running"` // Reason why not running, empty if leading
	Warmup     bool                 `json:"warmup"`
	Nodes      []StateNodeResponse  `json:"nodes"`
	Dangling   []integration.Config `json:"dangling"`
}

// StateNodeResponse is a chunk of StateResponse
type StateNodeResponse struct {
	Name    string               `json:"name"`
	Configs []integration.Config `json:"configs"`
}

// Stats holds statistics for the agent status command
type Stats struct {
	// Following
	Follower bool
	LeaderIP string

	// Leading
	Leader          bool
	Active          bool
	NodeCount       int
	ActiveConfigs   int
	DanglingConfigs int
	TotalConfigs    int
}
