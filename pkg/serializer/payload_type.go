// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serializer

// PayloadType identifies the kind of a metadata payload. It is sent to the intake in the
// `DD-Payload-Type` HTTP header so that payloads can be attributed without decompressing them.
//
// The list of values is bounded and append-only: a value is never renamed nor reused for a
// different payload. Adding one requires notifying the intake before the Agent release that ships it.
type PayloadType string

const (
	// PayloadTypeHeader is the HTTP header carrying the PayloadType of a metadata submission.
	PayloadTypeHeader = "DD-Payload-Type"

	// Payloads sent to /api/v1/metadata.

	// PayloadTypeAgentMetadata is the inventoryagent payload.
	PayloadTypeAgentMetadata PayloadType = "agent_metadata"
	// PayloadTypeHostMetadata is the inventoryhost payload.
	PayloadTypeHostMetadata PayloadType = "host_metadata"
	// PayloadTypeCheckMetadata is the inventorychecks payload.
	PayloadTypeCheckMetadata PayloadType = "check_metadata"
	// PayloadTypeHAAgentMetadata is the haagent payload.
	PayloadTypeHAAgentMetadata PayloadType = "ha_agent_metadata"
	// PayloadTypeHostGPUMetadata is the hostgpu payload.
	PayloadTypeHostGPUMetadata PayloadType = "host_gpu_metadata"
	// PayloadTypeHostSystemInfoMetadata is the hostsysteminfo payload.
	PayloadTypeHostSystemInfoMetadata PayloadType = "host_system_info_metadata"
	// PayloadTypeSigningMetadata is the packagesigning payload.
	PayloadTypeSigningMetadata PayloadType = "signing_metadata"
	// PayloadTypeSecurityAgentMetadata is the securityagent payload.
	PayloadTypeSecurityAgentMetadata PayloadType = "security_agent_metadata"
	// PayloadTypeSystemProbeMetadata is the systemprobe payload.
	PayloadTypeSystemProbeMetadata PayloadType = "system_probe_metadata"
	// PayloadTypeClusterAgentMetadata is the Cluster Agent clusteragent payload.
	PayloadTypeClusterAgentMetadata PayloadType = "datadog_cluster_agent_metadata"
	// PayloadTypeClusterCheckMetadata is the Cluster Agent clusterchecks payload.
	PayloadTypeClusterCheckMetadata PayloadType = "clustercheck_metadata"

	// Payloads sent to /intake/.

	// PayloadTypeAgentChecks is the agentchecks metadata payload.
	PayloadTypeAgentChecks PayloadType = "agent_checks"
	// PayloadTypeHostV5 is the v5 host metadata payload (gohai, resources).
	PayloadTypeHostV5 PayloadType = "host_v5"
	// PayloadTypeOTelHost is the OTel collector host metadata payload.
	PayloadTypeOTelHost PayloadType = "otel_host"
)
