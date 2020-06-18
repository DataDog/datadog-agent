// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package flavor

const (
	// DefaultAgentFlavor is the default Agent flavor
	DefaultAgentFlavor = "agent"
	// IotAgentFlavor is the IoT Agent flavor
	IotAgentFlavor = "iot_agent"
	// ClusterAgentFlavor is the Cluster Agent flavor
	ClusterAgentFlavor = "cluster_agent"
	// DogstatsdFlavor is the DogStatsD flavor
	DogstatsdFlavor = "dogstatsd"
	// SecurityAgentFlavor is the Security Agent flavor
	SecurityAgentFlavor = "security_agent"
)

// AgentFlavor is the running Agent flavor
var AgentFlavor string = DefaultAgentFlavor
