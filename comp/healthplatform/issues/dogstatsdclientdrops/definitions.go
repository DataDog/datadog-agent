// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package dogstatsdclientdrops

// ClientLibrary identifies a supported DogStatsD client telemetry library.
type ClientLibrary string

const (
	ClientLibraryGo     ClientLibrary = "go"
	ClientLibraryPython ClientLibrary = "py"
	ClientLibraryJava   ClientLibrary = "java"

	GoUDSIssueName = "DogStatsD Go UDS Client Payload Drops"
	GoUDSIssueType = "dogstatsd_go_uds_client_payload_drops"
	GoUDSIssueID   = "dogstatsd-go-uds-client-payload-drops"

	PythonUDSIssueName = "DogStatsD Python UDS Client Payload Drops"
	PythonUDSIssueType = "dogstatsd_python_uds_client_payload_drops"
	PythonUDSIssueID   = "dogstatsd-python-uds-client-payload-drops"

	JavaUDSIssueName = "DogStatsD Java UDS Client Payload Drops"
	JavaUDSIssueType = "dogstatsd_java_uds_client_payload_drops"
	JavaUDSIssueID   = "dogstatsd-java-uds-client-payload-drops"

	bytesDroppedMetric       = "datadog.dogstatsd.client.bytes_dropped"
	bytesDroppedQueueMetric  = "datadog.dogstatsd.client.bytes_dropped_queue"
	bytesDroppedWriterMetric = "datadog.dogstatsd.client.bytes_dropped_writer"

	highThroughputDocs = "https://docs.datadoghq.com/extend/dogstatsd/high_throughput/"
)

type issueDefinition struct {
	library             ClientLibrary
	displayName         string
	issueName           string
	issueType           string
	issueID             string
	dropReasonMetrics   []string
	remediationGuidance []string
}

var issueDefinitions = map[ClientLibrary]issueDefinition{
	ClientLibraryGo: {
		library:           ClientLibraryGo,
		displayName:       "Go",
		issueName:         GoUDSIssueName,
		issueType:         GoUDSIssueType,
		issueID:           GoUDSIssueID,
		dropReasonMetrics: []string{bytesDroppedQueueMetric, bytesDroppedWriterMetric},
		remediationGuidance: []string{
			"For queue drops, remove WithoutClientSideAggregation() if configured. If histogram, distribution, or timing traffic is significant, use WithExtendedClientSideAggregation(). Increase WithSenderQueueSize(...) only to absorb bursts.",
			"For writer drops, configure WithErrorHandler(statsd.LoggingErrorHandler) and inspect the UDS error. Increase WithWriteTimeout(...) only for write timeouts or WithConnectTimeout(...) only for connection timeouts.",
		},
	},
	ClientLibraryPython: {
		library:           ClientLibraryPython,
		displayName:       "Python",
		issueName:         PythonUDSIssueName,
		issueType:         PythonUDSIssueType,
		issueID:           PythonUDSIssueID,
		dropReasonMetrics: []string{bytesDroppedQueueMetric, bytesDroppedWriterMetric},
		remediationGuidance: []string{
			"Enable aggregation with disable_aggregation=False. When each process owns its DogStatsD client, enable buffering with disable_buffering=False. With datadog.initialize(), use statsd_disable_aggregation=False and statsd_disable_buffering=False instead.",
			"For queue drops, increase sender_queue_size or set a positive sender_queue_timeout.",
			"For writer drops, use buffering and aggregation to reduce UDS writes. Increase socket_timeout only for write timeouts; in datadogpy 0.53.0 or later, use socket_connect_timeout for transient connection failures.",
		},
	},
	ClientLibraryJava: {
		library:     ClientLibraryJava,
		displayName: "Java",
		issueName:   JavaUDSIssueName,
		issueType:   JavaUDSIssueType,
		issueID:     JavaUDSIssueID,
		remediationGuidance: []string{
			"Configure errorHandler(...) and inspect the reported UDS write error.",
			"For gauges, counts, and sets, confirm client-side aggregation is enabled with enableAggregation(true). Aggregation is enabled by default in version 3.0.0 and later.",
			"Increase timeout(...) only for write timeouts. For UDS stream connections, increase connectionTimeout(...) only for connection timeouts.",
		},
	},
}
