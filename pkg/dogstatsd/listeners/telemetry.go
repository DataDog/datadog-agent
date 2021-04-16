// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package listeners

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	// UDP
	tlmUDPPackets = telemetry.NewCounter("dogstatsd", "udp_packets",
		[]string{"state"}, "Dogstatsd UDP packets count")
	tlmUDPPacketsBytes = telemetry.NewCounter("dogstatsd", "udp_packets_bytes",
		nil, "Dogstatsd UDP packets bytes count")

	// UDS
	tlmUDSPackets = telemetry.NewCounter("dogstatsd", "uds_packets",
		[]string{"state"}, "Dogstatsd UDS packets count")
	tlmUDSOriginDetectionError = telemetry.NewCounter("dogstatsd", "uds_origin_detection_error",
		nil, "Dogstatsd UDS origin detection error count")
	tlmUDSPacketsBytes = telemetry.NewCounter("dogstatsd", "uds_packets_bytes",
		nil, "Dogstatsd UDS packets bytes")

	tlmListener            telemetry.Histogram
	defaultListenerBuckets = []float64{300, 500, 1000, 1500, 2000, 2500, 3000, 10000, 20000, 50000}
)

func init() {
	get := func(option string, defaultData []float64) []float64 {
		if !config.Datadog.IsSet(option) {
			return defaultData
		}

		buckets, err := config.Datadog.GetFloat64SliceE(option)
		if err != nil {
			log.Errorf("%s, falling back to default values", err)
			return defaultData
		}
		if len(buckets) == 0 {
			log.Debugf("'%s' is empty, falling back to default values", option)
			return defaultData
		}
		return buckets
	}

	tlmListener = telemetry.NewHistogram(
		"dogstatsd",
		"listener_read_latency",
		[]string{"listener_type"},
		"Time in nanoseconds while the listener is not reading data",
		get("telemetry.dogstatsd.listeners_latency_buckets", defaultListenerBuckets))
}
