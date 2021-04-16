// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packets

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	// packet buffer
	tlmBufferFlushedTimer = telemetry.NewCounter("dogstatsd", "packets_buffer_flush_timer",
		nil, "Count of packets buffer flush triggered by the timer")
	tlmBufferFlushedFull = telemetry.NewCounter("dogstatsd", "packets_buffer_flush_full",
		nil, "Count of packets buffer flush triggered because the buffer is full")
	tlmChannelSize = telemetry.NewGauge("dogstatsd", "packets_channel_size",
		nil, "Number of packets in the packets channel")

	// packet pool
	tlmPoolGet = telemetry.NewCounter("dogstatsd", "packet_pool_get",
		nil, "Count of get done in the packet pool")
	tlmPoolPut = telemetry.NewCounter("dogstatsd", "packet_pool_put",
		nil, "Count of put done in the packet pool")
	tlmPool = telemetry.NewGauge("dogstatsd", "packet_pool",
		nil, "Usage of the packet pool in dogstatsd")

	tlmListenerChannel    telemetry.Histogram
	defaultChannelBuckets = []float64{250, 500, 750, 1000, 10000}
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

	tlmListenerChannel = telemetry.NewHistogram(
		"dogstatsd",
		"listener_channel_latency",
		nil,
		"Time in nanoseconds to push a packets from a listeners to dogstatsd pipeline",
		get("telemetry.dogstatsd.listeners_channel_latency_buckets", defaultChannelBuckets))
}
