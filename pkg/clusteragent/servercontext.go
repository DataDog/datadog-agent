// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package clusteragent contains the functionality of the Cluster Agent.
package clusteragent

import (
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks"
)

// ConfigLister exposes check configs derived from DatadogInstrumentation CRs.
type ConfigLister interface {
	// ListConfigs returns all check configs and the state hash.
	ListConfigs() ([]integration.Config, uint64)
	// Hash returns a deterministic hash of the current instrumentation config state.
	Hash() uint64
}

// ServerContext holds business logic classes required to setup API endpoints
type ServerContext struct {
	ClusterCheckHandler *clusterchecks.Handler
}
