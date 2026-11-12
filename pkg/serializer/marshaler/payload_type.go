// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package marshaler

// MetadataPayloadType identifies the kind of a metadata payload. It is sent to the intake in the
// `DD-Payload-Type` HTTP header so that payloads can be attributed without decompressing them.
//
// The list of values is bounded and append-only: a value is never renamed nor reused for a
// different payload. Adding one requires notifying the intake before the Agent release that ships it.
type MetadataPayloadType string

const (
	// Payloads sent to /api/v1/metadata.

	// MetadataPayloadTypeAgent is the inventoryagent payload.
	MetadataPayloadTypeAgent MetadataPayloadType = "agent_metadata"
	// MetadataPayloadTypeHost is the inventoryhost payload.
	MetadataPayloadTypeHost MetadataPayloadType = "host_metadata"
	// MetadataPayloadTypeCheck is the inventorychecks payload.
	MetadataPayloadTypeCheck MetadataPayloadType = "check_metadata"
	// MetadataPayloadTypeHAAgent is the haagent payload.
	MetadataPayloadTypeHAAgent MetadataPayloadType = "ha_agent_metadata"
	// MetadataPayloadTypeHostGPU is the hostgpu payload.
	MetadataPayloadTypeHostGPU MetadataPayloadType = "host_gpu_metadata"
	// MetadataPayloadTypeHostSystemInfo is the hostsysteminfo payload.
	MetadataPayloadTypeHostSystemInfo MetadataPayloadType = "host_system_info_metadata"
	// MetadataPayloadTypeSigning is the packagesigning payload.
	MetadataPayloadTypeSigning MetadataPayloadType = "signing_metadata"
	// MetadataPayloadTypeSecurityAgent is the securityagent payload.
	MetadataPayloadTypeSecurityAgent MetadataPayloadType = "security_agent_metadata"
	// MetadataPayloadTypeSystemProbe is the systemprobe payload.
	MetadataPayloadTypeSystemProbe MetadataPayloadType = "system_probe_metadata"
	// MetadataPayloadTypeClusterAgent is the Cluster Agent clusteragent payload.
	MetadataPayloadTypeClusterAgent MetadataPayloadType = "datadog_cluster_agent_metadata"
	// MetadataPayloadTypeClusterCheck is the Cluster Agent clusterchecks payload.
	MetadataPayloadTypeClusterCheck MetadataPayloadType = "clustercheck_metadata"

	// Payloads sent to /intake/.

	// MetadataPayloadTypeAgentChecks is the agentchecks metadata payload.
	MetadataPayloadTypeAgentChecks MetadataPayloadType = "agent_checks"
	// MetadataPayloadTypeHostV5 is the v5 host metadata payload (gohai, resources).
	MetadataPayloadTypeHostV5 MetadataPayloadType = "host_v5"
	// MetadataPayloadTypeOTelHost is the OTel collector host metadata payload.
	MetadataPayloadTypeOTelHost MetadataPayloadType = "otel_host"
)

// TypedJSONMarshaler is a JSONMarshaler that also identifies the kind of metadata payload it carries.
// Metadata payloads implement it so the serializer can tag their submission with the matching type.
type TypedJSONMarshaler interface {
	JSONMarshaler
	// PayloadType returns the kind of this payload.
	PayloadType() MetadataPayloadType
}
