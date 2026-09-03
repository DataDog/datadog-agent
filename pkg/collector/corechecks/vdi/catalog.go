// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package vdi implements virtual desktop infrastructure monitoring on Windows.
package vdi

// CheckName is the VDI integration check name.
const CheckName = "vdi"

type metricType uint8

const (
	gaugeMetric metricType = iota
	monotonicCountMetric
)

type counterDefinition struct {
	counter  string
	metric   string
	kind     metricType
	optional bool
}

type objectDefinition struct {
	object   string
	prefix   string
	multiple bool
	counters []counterDefinition
}

func gauge(counter, metric string) counterDefinition {
	return counterDefinition{counter: counter, metric: metric, kind: gaugeMetric}
}

func monotonic(counter, metric string) counterDefinition {
	return counterDefinition{counter: counter, metric: metric, kind: monotonicCountMetric}
}

func optionalGauge(counter, metric string) counterDefinition {
	return counterDefinition{counter: counter, metric: metric, kind: gaugeMetric, optional: true}
}

func optionalMonotonic(counter, metric string) counterDefinition {
	return counterDefinition{counter: counter, metric: metric, kind: monotonicCountMetric, optional: true}
}

var serverCounters = []counterDefinition{
	gauge("Active Sessions", "active_sessions"),
	monotonic("Total Sessions", "total_sessions"),
	gauge("Active Connections", "active_connections"),
	monotonic("Total Connections", "total_connections"),
	monotonic("Idle Disconnections", "idle_disconnections"),
	optionalMonotonic("Ungraceful Disconnections", "ungraceful_disconnections"),
	gauge("Receive Rate bits/sec", "receive_rate"),
	monotonic("Received Bytes", "received_bytes"),
	gauge("Send Rate bits/sec", "send_rate"),
	monotonic("Sent Bytes", "sent_bytes"),
	gauge("HTTP Download Rate bits/sec", "http_download_rate"),
	monotonic("HTTP Downloaded Bytes", "http_downloaded_bytes"),
	gauge("Round-Trip Time ms", "round_trip_time"),
	gauge("Minimum Round-Trip Time ms", "minimum_round_trip_time"),
	monotonic("Total WebSocket Connections", "total_websocket_connections"),
	gauge("Active WebSocket Connections", "active_websocket_connections"),
	monotonic("Total QUIC Connections", "total_quic_connections"),
	gauge("Active QUIC Connections", "active_quic_connections"),
}

func cloneCounters(counters []counterDefinition) []counterDefinition {
	return append([]counterDefinition(nil), counters...)
}

var dcvObjects = []objectDefinition{
	{object: "DCV Server", prefix: "vdi.dcv.server", counters: cloneCounters(serverCounters)},
	{
		object: "DCV Server Processes", prefix: "vdi.dcv.process", multiple: true,
		counters: []counterDefinition{
			gauge("Process Identifier", "process_id"),
			gauge("% Processor Time", "cpu"),
			gauge("Physical Memory Bytes", "physical_memory"),
			gauge("Virtual Memory Bytes", "virtual_memory"),
		},
	},
	{
		object: "DCV Server Sessions", prefix: "vdi.dcv.session", multiple: true,
		counters: append([]counterDefinition{
			gauge("Session Duration sec", "duration"),
			gauge("Total Pixels", "total_pixels"),
			gauge("Display Count", "display_count"),
		}, cloneCounters(serverCounters[2:])...),
	},
	{
		object: "DCV Server Connections", prefix: "vdi.dcv.connection", multiple: true,
		counters: []counterDefinition{
			gauge("Connection Duration sec", "duration"),
			gauge("Receive Rate bits/sec", "receive_rate"),
			monotonic("Received Bytes", "received_bytes"),
			gauge("Send Rate bits/sec", "send_rate"),
			monotonic("Sent Bytes", "sent_bytes"),
			gauge("HTTP Download Rate bits/sec", "http_download_rate"),
			monotonic("HTTP Downloaded Bytes", "http_downloaded_bytes"),
			gauge("Round-Trip Time ms", "round_trip_time"),
			gauge("Minimum Round-Trip Time ms", "minimum_round_trip_time"),
		},
	},
	{
		object: "DCV Server Channels", prefix: "vdi.dcv.channel", multiple: true,
		counters: []counterDefinition{
			gauge("Receive Rate bits/sec", "receive_rate"),
			monotonic("Received Bytes", "received_bytes"),
			gauge("Send Rate bits/sec", "send_rate"),
			monotonic("Sent Bytes", "sent_bytes"),
		},
	},
	{
		object: "DCV Server Imaging", prefix: "vdi.dcv.imaging", multiple: true,
		counters: []counterDefinition{
			gauge("Grabbed Frames/sec", "grabbed_frames"),
			monotonic("Grabbed Frames", "grabbed_frames_total"),
			gauge("Sent Frames/sec", "sent_frames"),
			gauge("Dropped Frames/sec", "dropped_frames"),
			optionalGauge("Display Latency ms", "display_latency"),
			gauge("Available Bandwidth bits/sec", "available_bandwidth"),
			gauge("Encoded Frames/sec", "encoded_frames"),
			gauge("Encoding Time ms", "encoding_time"),
			gauge("Encoding Time per Megapixel ms", "encoding_time_per_megapixel"),
			gauge("Frame Quality %", "frame_quality"),
			gauge("Frame Compression Ratio %", "frame_compression_ratio"),
		},
	},
}
