// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package listeners

import (
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

var (
	// UDP
	tlmUDPPackets = telemetry.NewCounter("dogstatsd", "udp_packets",
		[]string{"state"}, "Dogstatsd UDP packets count")
	tlmUDPPacketsBytes = telemetry.NewCounter("dogstatsd", "udp_packets_bytes",
		nil, "Dogstatsd UDP packets bytes count")

	// UDS
	tlmUDSPackets = telemetry.NewCounter("dogstatsd", "uds_packets",
		[]string{"listener_id", "transport", "state"}, "Dogstatsd UDS packets count")
	tlmUDSOriginDetectionError = telemetry.NewCounter("dogstatsd", "uds_origin_detection_error",
		[]string{"listener_id", "transport"}, "Dogstatsd UDS origin detection error count")
	tlmUDSPacketsBytes = telemetry.NewCounter("dogstatsd", "uds_packets_bytes",
		[]string{"listener_id", "transport"}, "Dogstatsd UDS packets bytes")
	tlmUDSConnections = telemetry.NewGauge("dogstatsd", "uds_connections",
		[]string{"listener_id", "transport"}, "Dogstatsd UDS connections count")

	tlmListener            = telemetry.NewHistogramNoOp()
	defaultListenerBuckets = []float64{
		300, 500, 1000, // nanoseconds
		1500, 2_000, 2_500, 3_000, 10_000, 20_000, 50_000, 100_000, 200_000, 500_000, // milliseconds
		1_000_000, 2_000_000, 5_000_000, 10_000_000, 20_000_000, 50_000_000, 100_000_000, 200_000_000, 500_000_000, // milliseconds
	}
)

// InitTelemetry initialize the telemetry.Histogram buckets for the internal
// telemetry. This will be called once the first dogstatsd server is created
// since we need the configuration to be fully loaded.
func InitTelemetry(buckets []float64) {
	if buckets == nil {
		buckets = defaultListenerBuckets
	}

	tlmListener = telemetry.NewHistogram(
		"dogstatsd",
		"listener_read_latency",
		[]string{"listener_id", "transport", "listener_type"},
		"Time in nanoseconds while the listener is not reading data",
		buckets)
}
