// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package flavor defines the various flavors of the agent
package flavor

import pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"

const (
	// DefaultAgent is the default Agent flavor
	DefaultAgent = "agent"
	// IotAgent is the IoT Agent flavor
	IotAgent = "iot_agent"
	// ClusterAgent is the Cluster Agent flavor
	ClusterAgent = "cluster_agent"
	// Dogstatsd is the DogStatsD flavor
	Dogstatsd = "dogstatsd"
	// SecurityAgent is the Security Agent flavor
	SecurityAgent = "security_agent"
	// ServerlessAgent is an Agent running in a serverless environment
	ServerlessAgent = "serverless_agent"
	// HerokuAgent is the Heroku Agent flavor
	HerokuAgent = "heroku_agent"
	// ProcessAgent is the Process Agent flavor
	ProcessAgent = "process_agent"
	// TraceAgent is the Trace Agent flavor
	TraceAgent = "trace_agent"
)

var agentFlavors = map[string]string{
	DefaultAgent:    "Agent",
	IotAgent:        "IoT Agent",
	ClusterAgent:    "Cluster Agent",
	Dogstatsd:       "DogStatsD",
	SecurityAgent:   "Security Agent",
	ServerlessAgent: "Serverless Agent",
	HerokuAgent:     "Heroku Agent",
	ProcessAgent:    "Process Agent",
	TraceAgent:      "Trace Agent",
}

const unknownAgent = "Unknown Agent"

var agentFlavor = DefaultAgent

// SetFlavor sets the Agent flavor
func SetFlavor(flavor string) {
	agentFlavor = flavor

	if agentFlavor == IotAgent {
		pkgconfigsetup.Datadog.SetDefault("iot_host", true)
	}
}

// GetFlavor gets the running Agent flavor
// it MUST NOT be called before the main package is initialized;
// e.g. in init functions or to initialize package constants or variables.
func GetFlavor() string {
	return agentFlavor
}

// GetHumanReadableFlavor gets the running Agent flavor in a human readable form
func GetHumanReadableFlavor() string {
	if val, ok := agentFlavors[agentFlavor]; ok {
		return val
	}

	return unknownAgent
}
