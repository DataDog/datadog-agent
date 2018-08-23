// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package types

import "github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"

// NodeStatus holds the status report from the node-agent
type NodeStatus struct {
	LastChange int
}

// StatusResponse holds the DCA response for a status report
type StatusResponse struct {
	IsUpToDate bool
}

// ConfigResponse holds the DCA response for a config query
type ConfigResponse struct {
	LastChange int
	Configs    []integration.Config
}
