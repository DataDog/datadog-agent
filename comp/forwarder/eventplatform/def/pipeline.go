// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eventplatform

const (
	// ContentTypeJSON is the JSON payload content type for event-platform pipelines.
	ContentTypeJSON = "application/json"
	// ContentTypeProtobuf is the protobuf payload content type for event-platform pipelines.
	ContentTypeProtobuf = "application/x-protobuf"

	// DefaultBatchMaxConcurrentSend is the default HTTP batch max concurrent send.
	DefaultBatchMaxConcurrentSend = 0
	// DefaultBatchMaxSize is the default HTTP batch max size.
	DefaultBatchMaxSize = 1000
	// DefaultInputChanSize is the default input channel size.
	DefaultInputChanSize = 100
	// DefaultBatchMaxContentSize is the default HTTP batch max content size.
	DefaultBatchMaxContentSize = 5000000
)

// ExtraHTTPHeadersProvider returns additional HTTP headers for a pipeline.
type ExtraHTTPHeadersProvider func(hostname string) map[string]string

// PipelineDesc describes one event-platform passthrough pipeline.
//
// Product teams own these descriptors. The event-platform forwarder owns how
// descriptors are turned into running transport pipelines.
type PipelineDesc struct {
	EventType              string
	Category               string
	ContentType            string
	IntakeTrackType        string
	EndpointsConfigPrefix  string
	HostnameEndpointPrefix string

	DefaultBatchMaxConcurrentSend int
	DefaultBatchMaxContentSize    int
	DefaultBatchMaxSize           int
	DefaultInputChanSize          int

	ForceCompressionKind  string
	ForceCompressionLevel int
	UseStreamStrategy     bool
	ExtraHTTPHeaders      ExtraHTTPHeadersProvider

	SkipConnectivityDiagnose       bool
	SkipConnectivityDiagnoseReason string
}
