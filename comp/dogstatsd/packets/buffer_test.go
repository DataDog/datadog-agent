// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packets

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestBufferTelemetry(t *testing.T) {
	telemetryComponent := fxutil.Test[telemetry.Component](t, telemetryimpl.MockModule())
	telemetryStore := NewTelemetryStore(nil, telemetryComponent)
	// We need a high enough duration to avoid the buffer to flush
	// And cause the program to deadlock on the packetChannel
	duration := 10 * time.Second
	packetChannel := make(chan Packets)
	buffer := NewBuffer(3, duration, packetChannel, "test_buffer", telemetryStore)
	defer buffer.Close()

	packet := &Packet{
		Contents:   []byte("test"),
		Buffer:     []byte("test read"),
		Origin:     "test origin",
		ListenerID: "1",
		Source:     0,
	}

	buffer.Append(packet)
	buffer.Append(packet)

	telemetryMock, ok := telemetryComponent.(telemetry.Mock)
	assert.True(t, ok)

	bufferSizeBytesMetrics, err := telemetryMock.GetGaugeMetric("dogstatsd", "packets_buffer_size_bytes")
	assert.Nil(t, err)
	bufferSizeMetrics, err := telemetryMock.GetGaugeMetric("dogstatsd", "packets_buffer_size")
	assert.Nil(t, err)

	bufferSizeMetricLabel := bufferSizeMetrics[0].Labels()
	assert.Equal(t, bufferSizeMetricLabel["listener_id"], "test_buffer")
	assert.Equal(t, float64(2), bufferSizeMetrics[0].Value())

	bufferSizeBytesMetricLabel := bufferSizeBytesMetrics[0].Labels()
	assert.Equal(t, bufferSizeBytesMetricLabel["listener_id"], "test_buffer")
	assert.Equal(t, float64(246), bufferSizeBytesMetrics[0].Value())
}

func TestBufferTelemetryFull(t *testing.T) {
	telemetryComponent := fxutil.Test[telemetry.Component](t, telemetryimpl.MockModule())
	telemetryStore := NewTelemetryStore(nil, telemetryComponent)
	duration := 10 * time.Second
	packetChannel := make(chan Packets, 1)
	buffer := NewBuffer(0, duration, packetChannel, "test_buffer", telemetryStore)
	defer buffer.Close()

	packet := &Packet{
		Contents:   []byte("test"),
		Buffer:     []byte("test read"),
		Origin:     "test origin",
		ListenerID: "1",
		Source:     0,
	}

	buffer.Append(packet)

	telemetryMock, ok := telemetryComponent.(telemetry.Mock)
	assert.True(t, ok)

	bufferSizeBytesMetrics, err := telemetryMock.GetGaugeMetric("dogstatsd", "packets_buffer_size_bytes")
	assert.Nil(t, err)
	bufferSizeMetrics, err := telemetryMock.GetGaugeMetric("dogstatsd", "packets_buffer_size")
	assert.Nil(t, err)
	bufferFullMetrics, err := telemetryMock.GetCountMetric("dogstatsd", "packets_buffer_flush_full")
	assert.Nil(t, err)
	channelSizeMetrics, err := telemetryMock.GetGaugeMetric("dogstatsd", "packets_channel_size")
	assert.Nil(t, err)
	channelPacketsCountMetrics, err := telemetryMock.GetGaugeMetric("dogstatsd", "packets_channel_packets_count")
	assert.Nil(t, err)
	channelPacketsBytesMetrics, err := telemetryMock.GetGaugeMetric("dogstatsd", "packets_channel_packets_bytes")
	assert.Nil(t, err)

	// buffer size metrcis get reset when buffer is full
	bufferSizeMetricLabel := bufferSizeMetrics[0].Labels()
	assert.Equal(t, bufferSizeMetricLabel["listener_id"], "test_buffer")
	assert.Equal(t, float64(0), bufferSizeMetrics[0].Value())

	bufferSizeBytesMetricLabel := bufferSizeBytesMetrics[0].Labels()
	assert.Equal(t, bufferSizeBytesMetricLabel["listener_id"], "test_buffer")
	assert.Equal(t, float64(0), bufferSizeBytesMetrics[0].Value())

	bufferFullMetricLabel := bufferFullMetrics[0].Labels()
	assert.Equal(t, bufferFullMetricLabel["listener_id"], "test_buffer")
	assert.Equal(t, float64(1), bufferFullMetrics[0].Value())

	channelPacketsCountMetricLabel := channelPacketsCountMetrics[0].Labels()
	assert.Equal(t, channelPacketsCountMetricLabel["listener_id"], "test_buffer")
	assert.Equal(t, float64(1), channelPacketsCountMetrics[0].Value())

	channelPacketsBytesMetricLabel := channelPacketsBytesMetrics[0].Labels()
	assert.Equal(t, channelPacketsBytesMetricLabel["listener_id"], "test_buffer")
	assert.Equal(t, float64(123), channelPacketsBytesMetrics[0].Value())

	assert.Equal(t, float64(1), channelSizeMetrics[0].Value())
}
