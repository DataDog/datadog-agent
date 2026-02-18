// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

import "github.com/DataDog/datadog-agent/pkg/telemetry"

// Per-worker metrics
// TODO: right now pipeline name isn't associated with workers, but pipelines are always sent via a single worker. We can address this when
// we add per-pipeline metrics.
var (
	tlmWorkerStreamsOpened = telemetry.NewCounter("logs_sender_grpc_worker", "streams_opened", []string{"worker"}, "# Streams opened")
	tlmWorkerStreamErrors  = telemetry.NewCounter("logs_sender_grpc_worker", "stream_errors", []string{"worker", "reason"}, "Stream errors by reason")

	tlmWorkerBytesSent    = telemetry.NewCounter("logs_sender_grpc_worker", "bytes_sent", []string{"worker"}, "Bytes sent (compressed)")
	tlmWorkerBytesDropped = telemetry.NewCounter("logs_sender_grpc_worker", "bytes_dropped", []string{"worker"}, "Bytes dropped (compressed)")

	tlmWorkerInflightSize = telemetry.NewGauge("logs_sender_grpc_worker", "inflight_bytes", []string{"worker"}, "Gauge of current serialized inflight bytes for the pipeline")
)
