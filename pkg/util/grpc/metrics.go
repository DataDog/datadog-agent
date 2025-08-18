// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package grpc

import (
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

// gRPC metrics following the specified schema
var (
	// requestCount tracks total number of gRPC requests processed
	requestCount = telemetry.NewCounter("grpc", "request_count",
		[]string{"service_method", "peer", "status"}, "Total number of gRPC requests processed")

	// errorCount tracks total number of gRPC errors encountered
	errorCount = telemetry.NewCounter("grpc", "error_count",
		[]string{"service_method", "peer", "error_code"}, "Total number of gRPC errors encountered")

	// requestDuration tracks distribution of gRPC request latencies
	requestDuration = telemetry.NewHistogram("grpc", "request_duration_seconds",
		[]string{"service_method", "peer"}, "Distribution of gRPC request latencies",
		[]float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10, 30, 60})

	// payloadSize tracks distribution of payload sizes for gRPC calls
	payloadSize = telemetry.NewHistogram("grpc", "payload_size_bytes",
		[]string{"service_method", "peer", "direction"}, "Distribution of payload sizes for gRPC calls",
		[]float64{100, 500, 1000, 5000, 10000, 50000, 100000, 500000, 1000000, 5000000, 10000000})

	// activeRequests tracks number of currently active (in-flight) requests
	activeRequests = telemetry.NewGauge("grpc", "active_requests",
		[]string{"service_method", "peer"}, "Number of currently active (in-flight) requests")
)
