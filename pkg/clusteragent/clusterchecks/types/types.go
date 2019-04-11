// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package types

import (
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"k8s.io/apimachinery/pkg/types"
	v1 "k8s.io/client-go/listers/core/v1"
)

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

// LeaderIPCallback describes the leader-election method we
// need and allows to inject a custom one for tests
type LeaderIPCallback func() (string, error)

// Listers are used to list kube services and endpoints
type Listers struct {
	ServicesLister  v1.ServiceLister
	EndpointsLister v1.EndpointsLister
}

// ServiceInfo is used to store data needed to dispatch endpoints checks
type ServiceInfo struct {
	CheckName  string
	Instances  []integration.Data
	InitConfig integration.Data
	Namespace  string
	Name       string
	ClusterIP  string
	Entity     string
}

// ServicesCache is used to cache data of scheduled kube service checks
// needed to dispatch endpoints checks
// UpdateNeeded is used to optimize updating the cache, should be set to true
// if a new kube service check is scheduled
type ServicesCache struct {
	Service      map[types.UID]*ServiceInfo
	UpdateNeeded bool
}

// EndpointInfo is used to store the collected info of an endpoint
// used to create config for endpoints
type EndpointInfo struct {
	PodUID        types.UID
	ServiceEntity string
	IP            string // endpoint's IP
	Ports         []int32
	CheckName     string
	ClusterIP     string // correspendent service's IP
	Instances     []integration.Data
	InitConfig    integration.Data
}
