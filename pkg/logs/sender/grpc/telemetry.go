// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

import "github.com/DataDog/datadog-agent/pkg/telemetry"

var (
	// Per-pipeline
	tlmPipelineStateSize           = telemetry.NewGauge("logs_sender_grpc_pipeline", "state_size_bytes", []string{"pipeline"}, "Gauge of current serialized state size for the pipeline")
	tlmPipelinePatternsAdded       = telemetry.NewCounter("logs_sender_grpc_pipeline", "patterns_added", []string{"pipeline"}, "Number of patterns added to state")
	tlmPipelinePatternsRemoved     = telemetry.NewCounter("logs_sender_grpc_pipeline", "patterns_removed", []string{"pipeline"}, "Number of patterns removed from state")
	tlmPipelineTokensAdded         = telemetry.NewCounter("logs_sender_grpc_pipeline", "tokens_added", []string{"pipeline"}, "Number of dictionary entries added to state")
	tlmPipelineTokensRemoved       = telemetry.NewCounter("logs_sender_grpc_pipeline", "tokens_removed", []string{"pipeline"}, "Number of dictionary entries removed from state")
	tlmPipelinePatternBytesAdded   = telemetry.NewCounter("logs_sender_grpc_pipeline", "pattern_bytes_added", []string{"pipeline"}, "Bytes of pattern definitions added to state")
	tlmPipelinePatternBytesRemoved = telemetry.NewCounter("logs_sender_grpc_pipeline", "pattern_bytes_removed", []string{"pipeline"}, "Bytes of pattern definitions removed from state")
	tlmPipelineTokenBytesAdded     = telemetry.NewCounter("logs_sender_grpc_pipeline", "token_bytes_added", []string{"pipeline"}, "Bytes of dictionary entries added to state")
	tlmPipelineTokenBytesRemoved   = telemetry.NewCounter("logs_sender_grpc_pipeline", "token_bytes_removed", []string{"pipeline"}, "Bytes of dictionary entries removed from state")
	tlmPipelinePatternLogBytes     = telemetry.NewCounter("logs_sender_grpc_pipeline", "pattern_log_bytes", []string{"pipeline"}, "Bytes of pattern (structured) logs sent")
	tlmPipelineRawLogBytes         = telemetry.NewCounter("logs_sender_grpc_pipeline", "raw_log_bytes", []string{"pipeline"}, "Bytes of raw logs sent")
	tlmPipelineStateChangeBytes    = telemetry.NewCounter("logs_sender_grpc_pipeline", "state_change_bytes", []string{"pipeline"}, "Bytes of state changes sent")

	// Per-stream
	tlmStreamPatternLogs      = telemetry.NewCounter("logs_sender_grpc_stream", "pattern_logs_sent", []string{"stream"}, "Number of pattern (structured) logs sent")
	tlmStreamPatternLogBytes  = telemetry.NewCounter("logs_sender_grpc_stream", "pattern_log_bytes", []string{"stream"}, "Bytes of pattern (structured) logs sent")
	tlmStreamStateChanges     = telemetry.NewCounter("logs_sender_grpc_stream", "state_changes_sent", []string{"stream"}, "Number of state changes sent (pattern/dict add/remove)")
	tlmStreamStateChangeBytes = telemetry.NewCounter("logs_sender_grpc_stream", "state_change_bytes", []string{"stream"}, "Bytes of state changes sent")
	tlmStreamBatches          = telemetry.NewCounter("logs_sender_grpc_stream", "batches_sent", []string{"stream"}, "Number of batches sent on the stream")
	tlmStreamDatumsPerBatch   = telemetry.NewHistogram("logs_sender_grpc_stream", "datums_per_batch", []string{"stream"}, "Histogram of datums per batch", []float64{1, 2, 4, 8, 16, 32, 64, 128, 256})
)
