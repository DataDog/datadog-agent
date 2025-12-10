// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

import "github.com/DataDog/datadog-agent/pkg/telemetry"

// TODO: right now we aren't using pkg/logs/metrics. Determine which metrics can be shared.

// Per-pipeline Metrics
// Currently bytes metrics use proto.Size() of the Datum message that encodes the state change/log.
var (
	tlmPipelineStateSize = telemetry.NewGauge("logs_sender_grpc", "state_size_bytes", []string{"pipeline"}, "Gauge of current serialized state size for the pipeline")

	tlmPipelinePatternAdded        = telemetry.NewCounter("logs_sender_grpc", "patterns_added", []string{"pipeline"}, "Number of patterns added to state")
	tlmPipelinePatternBytesAdded   = telemetry.NewCounter("logs_sender_grpc", "pattern_bytes_added", []string{"pipeline"}, "Bytes of pattern definitions added to state")
	tlmPipelinePatternRemoved      = telemetry.NewCounter("logs_sender_grpc", "patterns_removed", []string{"pipeline"}, "Number of patterns removed from state")
	tlmPipelinePatternBytesRemoved = telemetry.NewCounter("logs_sender_grpc", "pattern_bytes_removed", []string{"pipeline"}, "Bytes of pattern definitions removed from state")

	tlmPipelineTokenAdded      = telemetry.NewCounter("logs_sender_grpc", "tokens_added", []string{"pipeline"}, "Number of dictionary entries added to state")
	tlmPipelineTokenBytesAdded = telemetry.NewCounter("logs_sender_grpc", "token_bytes_added", []string{"pipeline"}, "Bytes of dictionary entries added to state")
	// TODO: tokens are not yet evicted.
	_ = telemetry.NewCounter("logs_sender_grpc", "tokens_removed", []string{"pipeline"}, "Number of dictionary entries removed from state")
	_ = telemetry.NewCounter("logs_sender_grpc", "token_bytes_removed", []string{"pipeline"}, "Bytes of dictionary entries removed from state")

	tlmPipelinePatternLogsProcessed      = telemetry.NewCounter("logs_sender_grpc", "pattern_logs", []string{"pipeline"}, "# Patterned logs processed")
	tlmPipelinePatternLogsProcessedBytes = telemetry.NewCounter("logs_sender_grpc", "pattern_logs_bytes", []string{"pipeline"}, "Bytes of patterned logs sent")
	tlmPipelineRawLogsProcessed          = telemetry.NewCounter("logs_sender_grpc", "raw_logs", []string{"pipeline"}, "# raw logs sent")
	tlmPipelineRawLogsProcessedBytes     = telemetry.NewCounter("logs_sender_grpc", "raw_logs_bytes", []string{"pipeline"}, "Bytes of raw logs sent")
)

// Per-worker metrics
// TODO: right now pipeline name isn't associated with workers, but pipelines are 1:1 with workers
var (
	tlmWorkerStreamsOpened = telemetry.NewCounter("logs_sender_grpc_worker", "streams_opened", []string{"worker"}, "# Streams opened")
	tlmWorkerStreamErrors  = telemetry.NewCounter("logs_sender_grpc_worker", "stream_errors", []string{"worker", "reason"}, "Stream errors by reason")

	tlmWorkerBytesSent    = telemetry.NewCounter("logs_sender_grpc_worker", "bytes_sent", []string{"worker"}, "Bytes sent (compressed)")
	tlmWorkerBytesDropped = telemetry.NewCounter("logs_sender_grpc_worker", "bytes_dropped", []string{"worker"}, "Bytes dropped (compressed)")

	tlmWorkerInflightSize = telemetry.NewGauge("logs_sender_grpc_worker", "inflight_bytes", []string{"worker"}, "Gauge of current serialized inflight bytes for the pipeline")
)

// TODO: use TlmSenderLatency (or similar metric) to track time to ack.
