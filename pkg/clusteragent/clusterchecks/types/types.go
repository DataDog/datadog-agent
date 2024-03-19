// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package types implements the types used by the Cluster checks dispatching
// functionality.
package types

import (
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

const (
	// ExtraHeartbeatLastChangeValue is used to instruct the Cluster Agent that we're still alive
	// despite that the polling loop on CLC side is delayed.
	ExtraHeartbeatLastChangeValue int64 = -1
)

// NodeStatus holds the status report from the node-agent
type NodeStatus struct {
	LastChange int64 `json:"last_change"`
}

// StatusResponse holds the DCA response for a status report
type StatusResponse struct {
	IsUpToDate bool `json:"isuptodate"`
}

// RebalanceResponse holds the DCA response for a rebalancing request
type RebalanceResponse struct {
	CheckID     string `json:"check_id"`
	CheckWeight int    `json:"check_weight"`

	SourceNodeName string `json:"source_node_name"`
	SourceDiff     int    `json:"source_diff"`

	DestNodeName string `json:"dest_node_name"`
	DestDiff     int    `json:"dest_diff"`
}

// IsolateResponse holds the DCA response for an isolate request
type IsolateResponse struct {
	CheckID    string `json:"check_id"`
	CheckNode  string `json:"check_node"`
	IsIsolated bool   `json:"is_isolated"`
	Reason     string `json:"reason"`
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
	CheckNames      map[string]struct{}
}

// LeaderIPCallback describes the leader-election method we
// need and allows to inject a custom one for tests
type LeaderIPCallback func() (string, error)

// CLCRunnersStats is used to unmarshall the CLC Runners stats payload
type CLCRunnersStats map[string]CLCRunnerStats

// CLCRunnerStats is used to unmarshall the stats of each CLC Runner
type CLCRunnerStats struct {
	AverageExecutionTime int  `json:"AverageExecutionTime"`
	MetricSamples        int  `json:"MetricSamples"`
	HistogramBuckets     int  `json:"HistogramBuckets"`
	Events               int  `json:"Events"`
	IsClusterCheck       bool `json:"IsClusterCheck"`
	LastExecFailed       bool `json:"LastExecFailed"`
}

// Workers is used to unmarshal the workers info of each CLC Runner
type Workers struct {
	Count     int                   `json:"Count"`
	Instances map[string]WorkerInfo `json:"Instances"`
}

// WorkerInfo is used to unmarshal the utilization of each worker
type WorkerInfo struct {
	Utilization float64 `json:"Utilization"`
}
