// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packets

import (
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
)

var (
	defaultChannelBuckets = []float64{250, 500, 750, 1000, 10000}
)

// TelemetryStore holds all the telemetry counters and gauges for the dogstatsd packets
type TelemetryStore struct {
	// packet buffer
	// This is the size of the output channel of the packet buffer. Even
	// though all buffers currently share a single channel it's still worth
	// tagging it with listener_id in case this changes later.
	tlmChannelSize             telemetry.Gauge
	tlmChannelSizePackets      telemetry.Gauge
	tlmChannelSizePacketsBytes telemetry.Gauge

	tlmListenerChannel telemetry.Histogram
	// buffer flush
	tlmBufferFlushedTimer telemetry.Counter
	tlmBufferFlushedFull  telemetry.Counter
	tlmBufferSize         telemetry.Gauge
	tlmBufferSizeBytes    telemetry.Gauge

	// packet pool
	tlmPoolGet telemetry.Counter
	tlmPoolPut telemetry.Counter
	tlmPool    telemetry.Gauge
}

// NewTelemetryStore returns a new TelemetryStore
func NewTelemetryStore(buckets []float64, telemetrycomp telemetry.Component) *TelemetryStore {
	if buckets == nil {
		buckets = defaultChannelBuckets
	}

	return &TelemetryStore{
		tlmChannelSize: telemetrycomp.NewGauge("dogstatsd", "packets_channel_size",
			[]string{}, "Size of the packets channel (batch of packets)"),
		tlmChannelSizePackets: telemetrycomp.NewGauge("dogstatsd", "packets_channel_packets_count",
			[]string{"listener_id"}, "Number of packets in the packets channel"),
		tlmChannelSizePacketsBytes: telemetrycomp.NewGauge("dogstatsd", "packets_channel_packets_bytes",
			[]string{"listener_id"}, "Number of bytes in the packets channel"),

		tlmListenerChannel: telemetrycomp.NewHistogram(
			"dogstatsd",
			"listener_channel_latency",
			[]string{"listener_id"},
			"Time in nanoseconds to push a packets from a listeners to dogstatsd pipeline",
			buckets),

		tlmBufferFlushedTimer: telemetrycomp.NewCounter("dogstatsd", "packets_buffer_flush_timer",
			[]string{"listener_id"}, "Count of packets buffer flush triggered by the timer"),
		tlmBufferFlushedFull: telemetrycomp.NewCounter("dogstatsd", "packets_buffer_flush_full",
			[]string{"listener_id"}, "Count of packets buffer flush triggered because the buff,er is full"),
		tlmBufferSize: telemetrycomp.NewGauge("dogstatsd", "packets_buffer_size",
			[]string{"listener_id"}, "Size of the packets buffer"),
		tlmBufferSizeBytes: telemetrycomp.NewGauge("dogstatsd", "packets_buffer_size_bytes",
			[]string{"listener_id"}, "Size of the packets buffer in bytes"),

		tlmPoolGet: telemetrycomp.NewCounter("dogstatsd", "packet_pool_get",
			nil, "Count of get done in the packet pool"),
		tlmPoolPut: telemetrycomp.NewCounter("dogstatsd", "packet_pool_put",
			nil, "Count of put done in the packet pool"),
		tlmPool: telemetrycomp.NewGauge("dogstatsd", "packet_pool",
			nil, "Usage of the packet pool in dogstatsd"),
	}
}

// TelemetryTrackPackets tracks the number of packets in the channel and the number of bytes
func (t *TelemetryStore) TelemetryTrackPackets(packets Packets, listenerID string) {
	t.tlmChannelSizePackets.Add(float64(len(packets)), listenerID)
	t.tlmChannelSizePacketsBytes.Add(float64(packets.SizeInBytes()+packets.DataSizeInBytes()), listenerID)
}

// TelemetryUntrackPackets untracks the number of packets in the channel and the number of bytes
func (t *TelemetryStore) TelemetryUntrackPackets(packets Packets) {
	for _, packet := range packets {
		t.tlmChannelSizePackets.Add(-1, packet.ListenerID)
		t.tlmChannelSizePacketsBytes.Add(-float64(packet.SizeInBytes()+packet.DataSizeInBytes()), packet.ListenerID)
	}
}
