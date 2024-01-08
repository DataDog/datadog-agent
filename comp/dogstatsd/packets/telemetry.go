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
	// This is the size of the output channel of the packet buffer. Even
	// though all buffers currently share a single channel it's still worth
	// tagging it with listener_id in case this changes later.
	tlmChannelSize = telemetry.NewGauge("dogstatsd", "packets_channel_size",
		[]string{}, "Size of the packets channel (batch of packets)")
	tlmChannelSizePackets = telemetry.NewGauge("dogstatsd", "packets_channel_packets_count",
		[]string{"listener_id"}, "Number of packets in the packets channel")
	tlmChannelSizePacketsBytes = telemetry.NewGauge("dogstatsd", "packets_channel_packets_bytes",
		[]string{"listener_id"}, "Number of bytes in the packets channel")

	tlmListenerChannel    = telemetry.NewHistogramNoOp()
	defaultChannelBuckets = []float64{250, 500, 750, 1000, 10000}

	// buffer flush
	tlmBufferFlushedTimer = telemetry.NewCounter("dogstatsd", "packets_buffer_flush_timer",
		[]string{"listener_id"}, "Count of packets buffer flush triggered by the timer")
	tlmBufferFlushedFull = telemetry.NewCounter("dogstatsd", "packets_buffer_flush_full",
		[]string{"listener_id"}, "Count of packets buffer flush triggered because the buffer is full")
	tlmBufferSize = telemetry.NewGauge("dogstatsd", "packets_buffer_size",
		[]string{"listener_id"}, "Size of the packets buffer")
	tlmBufferSizeBytes = telemetry.NewGauge("dogstatsd", "packets_buffer_size_bytes",
		[]string{"listener_id"}, "Size of the packets buffer in bytes")

	// packet pool
	tlmPoolGet = telemetry.NewCounter("dogstatsd", "packet_pool_get",
		nil, "Count of get done in the packet pool")
	tlmPoolPut = telemetry.NewCounter("dogstatsd", "packet_pool_put",
		nil, "Count of put done in the packet pool")
	tlmPool = telemetry.NewGauge("dogstatsd", "packet_pool",
		nil, "Usage of the packet pool in dogstatsd")
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
		[]string{"listener_id"},
		"Time in nanoseconds to push a packets from a listeners to dogstatsd pipeline",
		buckets)
}

// TelemetryTrackPackets tracks the number of packets in the channel and the number of bytes
func TelemetryTrackPackets(packets Packets, listenerID string) {
	tlmChannelSizePackets.Add(float64(len(packets)), listenerID)
	tlmChannelSizePacketsBytes.Add(float64(packets.SizeInBytes()+packets.DataSizeInBytes()), listenerID)
}

// TelemetryUntrackPackets untracks the number of packets in the channel and the number of bytes
func TelemetryUntrackPackets(packets Packets) {
	for _, packet := range packets {
		tlmChannelSizePackets.Add(-1, packet.ListenerID)
		tlmChannelSizePacketsBytes.Add(-float64(packet.SizeInBytes()+packet.DataSizeInBytes()), packet.ListenerID)
	}
}
