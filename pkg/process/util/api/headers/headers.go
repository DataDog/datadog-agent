// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package headers

const (
	// HostHeader contains the hostname of the payload
	HostHeader = "X-Dd-Hostname"
	// ContainerCountHeader contains the container count in the payload
	ContainerCountHeader = "X-Dd-ContainerCount"
	// ProcessVersionHeader holds the process agent version sending the payload
	ProcessVersionHeader = "X-Dd-Processagentversion"
	// ClusterIDHeader contains the orchestrator cluster ID of this agent
	ClusterIDHeader = "X-Dd-Orchestrator-ClusterID"
	// TimestampHeader contains the timestamp that the check data was created
	TimestampHeader = "X-DD-Agent-Timestamp"
)
