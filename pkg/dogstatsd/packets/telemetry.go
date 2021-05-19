// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packets

import (
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

var (
	// packet buffer
	tlmChannelSize = telemetry.NewGauge("dogstatsd", "packets_channel_size",
		nil, "Number of packets in the packets channel")

	tlmListenerChannel    = telemetry.NewHistogramNoOp()
	defaultChannelBuckets = []float64{250, 500, 750, 1000, 10000}
)

// InitTelemetry initialize the telemetry.Histogram buckets for the internal
// telemetry. This will be called once the first dogstatsd server is created
// since we need the configuration to be fully loaded.
func InitTelemetry(buckets []float64) {
	if buckets == nil {
		buckets = defaultChannelBuckets
	}

	tlmListenerChannel = telemetry.NewHistogram(
		"dogstatsd",
		"listener_channel_latency",
		nil,
		"Time in nanoseconds to push a packets from a listeners to dogstatsd pipeline",
		buckets)
}
