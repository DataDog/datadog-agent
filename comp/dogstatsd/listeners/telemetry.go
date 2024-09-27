// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package listeners

import (
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
)

var (
	defaultListenerBuckets = []float64{300, 500, 1000, 1500, 2000, 2500, 3000, 10000, 20000, 50000}
)

// TelemetryStore holds all the telemetry counters and gauges for the dogstatsd listeners
type TelemetryStore struct {
	// UDP
	tlmUDPPackets      telemetry.Counter
	tlmUDPPacketsBytes telemetry.Counter
	// UDS
	tlmUDSPackets              telemetry.Counter
	tlmUDSOriginDetectionError telemetry.Counter
	tlmUDSPacketsBytes         telemetry.Counter
	tlmUDSConnections          telemetry.Gauge

	tlmListener telemetry.Histogram
}

// NewTelemetryStore returns a new TelemetryStore
func NewTelemetryStore(buckets []float64, telemetrycomp telemetry.Component) *TelemetryStore {
	if buckets == nil {
		buckets = defaultListenerBuckets
	}

	return &TelemetryStore{
		tlmUDPPackets: telemetrycomp.NewCounter("dogstatsd", "udp_packets",
			[]string{"state"}, "Dogstatsd UDP packets count"),
		tlmUDPPacketsBytes: telemetrycomp.NewCounter("dogstatsd", "udp_packets_bytes",
			nil, "Dogstatsd UDP packets bytes count"),
		tlmUDSPackets: telemetrycomp.NewCounter("dogstatsd", "uds_packets",
			[]string{"listener_id", "transport", "state"}, "Dogstatsd UDS packets count"),
		tlmUDSOriginDetectionError: telemetrycomp.NewCounter("dogstatsd", "uds_origin_detection_error",
			[]string{"listener_id", "transport"}, "Dogstatsd UDS origin detection error count"),
		tlmUDSPacketsBytes: telemetrycomp.NewCounter("dogstatsd", "uds_packets_bytes",
			[]string{"listener_id", "transport"}, "Dogstatsd UDS packets bytes"),
		tlmUDSConnections: telemetrycomp.NewGauge("dogstatsd", "uds_connections",
			[]string{"listener_id", "transport"}, "Dogstatsd UDS connections count"),
		tlmListener: telemetrycomp.NewHistogram(
			"dogstatsd",
			"listener_read_latency",
			[]string{"listener_id", "transport", "listener_type"},
			"Time in nanoseconds while the listener is not reading data",
			buckets),
	}

}
