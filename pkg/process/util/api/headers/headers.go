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
	// ProtobufContentType contains that the content type is protobuf
	ProtobufContentType = "application/x-protobuf"
	// ContentTypeHeader contains the content type of the payload
	ContentTypeHeader = "Content-Type"
	// EVPOriginHeader is the source/origin sending a request to the intake. This field should be filled with the name of the library sending profiles.
	EVPOriginHeader = "DD-EVP-ORIGIN"
	// EVPOriginVersionHeader is the version of above origin
	EVPOriginVersionHeader = "DD-EVP-ORIGIN-VERSION"
	// ContentEncodingHeader contains the encoding type of the payload
	ContentEncodingHeader = "Content-Encoding"
	// ZSTDContentEncoding contains that the encoding type is zstd
	ZSTDContentEncoding = "zstd"
	// RequestIDHeader contains a unique identifier per payloads being sent to the intake servers
	RequestIDHeader = "X-DD-Request-ID"
	// AgentStartTime contains the timestamp that the agent was started
	AgentStartTime = "X-DD-Agent-Start-Time"
)
