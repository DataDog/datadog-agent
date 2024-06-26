// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packets

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	dto "github.com/prometheus/client_model/go"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestBufferTelemetry(t *testing.T) {
	telemetryComponent := fxutil.Test[telemetry.Component](t, telemetryimpl.MockModule())
	telemetryStore := NewTelemetryStore(nil, telemetryComponent)
	// We need a high enough duration to avoid the buffer to flush
	// And cause the program to deadlock on the packetChannel
	duration, _ := time.ParseDuration("10s")
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

	var bufferSizeBytesMetric []*dto.Metric
	var bufferSizeMetric []*dto.Metric
	metricsFamily, err := telemetryMock.GetRegistry().Gather()
	assert.Nil(t, err)

	for _, metric := range metricsFamily {
		if metric.GetName() == "dogstatsd__packets_buffer_size" {
			bufferSizeMetric = metric.GetMetric()
		}

		if metric.GetName() == "dogstatsd__packets_buffer_size_bytes" {
			bufferSizeBytesMetric = metric.GetMetric()
		}
	}

	assert.NotNil(t, bufferSizeBytesMetric)
	assert.NotNil(t, bufferSizeMetric)

	bufferSizeMetricLabel := bufferSizeMetric[0].GetLabel()[0]
	assert.Equal(t, "listener_id", bufferSizeMetricLabel.GetName())
	assert.Equal(t, "test_buffer", bufferSizeMetricLabel.GetValue())
	assert.Equal(t, float64(2), bufferSizeMetric[0].GetGauge().GetValue())

	bufferSizeBytesMetricLabel := bufferSizeBytesMetric[0].GetLabel()[0]
	assert.Equal(t, "listener_id", bufferSizeBytesMetricLabel.GetName())
	assert.Equal(t, "test_buffer", bufferSizeBytesMetricLabel.GetValue())
	assert.Equal(t, float64(246), bufferSizeBytesMetric[0].GetGauge().GetValue())
}

func TestBufferTelemetryFull(t *testing.T) {
	telemetryComponent := fxutil.Test[telemetry.Component](t, telemetryimpl.MockModule())
	telemetryStore := NewTelemetryStore(nil, telemetryComponent)
	duration, _ := time.ParseDuration("10s")
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

	registry := telemetryMock.GetRegistry()
	var bufferSizeBytesMetric []*dto.Metric
	var bufferSizeMetric []*dto.Metric
	var bufferFullMetric []*dto.Metric
	var channelSizeMetric []*dto.Metric
	var channelPacketsCountMetric []*dto.Metric
	var channelPacketsBytesMetric []*dto.Metric

	metricsFamily, err := registry.Gather()
	assert.Nil(t, err)

	for _, metric := range metricsFamily {
		if metric.GetName() == "dogstatsd__packets_buffer_size" {
			bufferSizeMetric = metric.GetMetric()
		}

		if metric.GetName() == "dogstatsd__packets_buffer_size_bytes" {
			bufferSizeBytesMetric = metric.GetMetric()
		}

		if metric.GetName() == "dogstatsd__packets_buffer_flush_full" {
			bufferFullMetric = metric.GetMetric()
		}

		if metric.GetName() == "dogstatsd__packets_channel_size" {
			channelSizeMetric = metric.GetMetric()
		}

		if metric.GetName() == "dogstatsd__packets_channel_packets_count" {
			channelPacketsCountMetric = metric.GetMetric()
		}

		if metric.GetName() == "dogstatsd__packets_channel_packets_bytes" {
			channelPacketsBytesMetric = metric.GetMetric()
		}
	}

	assert.NotNil(t, bufferSizeBytesMetric)
	assert.NotNil(t, bufferSizeMetric)
	assert.NotNil(t, bufferFullMetric)
	assert.NotNil(t, channelSizeMetric)
	assert.NotNil(t, channelPacketsCountMetric)
	assert.NotNil(t, channelPacketsBytesMetric)

	// buffer size metrcis get reset when buffer is full
	bufferSizeMetricLabel := bufferSizeMetric[0].GetLabel()[0]
	assert.Equal(t, "listener_id", bufferSizeMetricLabel.GetName())
	assert.Equal(t, "test_buffer", bufferSizeMetricLabel.GetValue())
	assert.Equal(t, float64(0), bufferSizeMetric[0].GetGauge().GetValue())

	bufferSizeBytesMetricLabel := bufferSizeBytesMetric[0].GetLabel()[0]
	assert.Equal(t, "listener_id", bufferSizeBytesMetricLabel.GetName())
	assert.Equal(t, "test_buffer", bufferSizeBytesMetricLabel.GetValue())
	assert.Equal(t, float64(0), bufferSizeBytesMetric[0].GetGauge().GetValue())

	bufferFullMetricLabel := bufferFullMetric[0].GetLabel()[0]
	assert.Equal(t, "listener_id", bufferFullMetricLabel.GetName())
	assert.Equal(t, "test_buffer", bufferFullMetricLabel.GetValue())
	assert.Equal(t, float64(1), bufferFullMetric[0].GetCounter().GetValue())

	channelPacketsCountMetricLabel := channelPacketsCountMetric[0].GetLabel()[0]
	assert.Equal(t, "listener_id", channelPacketsCountMetricLabel.GetName())
	assert.Equal(t, "test_buffer", channelPacketsCountMetricLabel.GetValue())
	assert.Equal(t, float64(1), channelPacketsCountMetric[0].GetGauge().GetValue())

	channelPacketsBytesMetricLabel := channelPacketsBytesMetric[0].GetLabel()[0]
	assert.Equal(t, "listener_id", channelPacketsBytesMetricLabel.GetName())
	assert.Equal(t, "test_buffer", channelPacketsBytesMetricLabel.GetValue())
	assert.Equal(t, float64(123), channelPacketsBytesMetric[0].GetGauge().GetValue())

	assert.Equal(t, float64(1), channelSizeMetric[0].GetGauge().GetValue())
}

func TestBufferFLushLoopTelemetryFull(t *testing.T) {
	telemetryComponent := fxutil.Test[telemetry.Component](t, telemetryimpl.MockModule())
	telemetryStore := NewTelemetryStore(nil, telemetryComponent)
	duration, _ := time.ParseDuration("1ns")

	buffer := NewBuffer(0, duration, nil, "test_buffer", telemetryStore)
	defer buffer.Close()

	telemetryMock, ok := telemetryComponent.(telemetry.Mock)
	assert.True(t, ok)

	var bufferFlushTimerMetric []*dto.Metric
	metricsFamily, err := telemetryMock.GetRegistry().Gather()
	assert.Nil(t, err)

	for _, metric := range metricsFamily {
		if metric.GetName() == "dogstatsd__packets_buffer_flush_timer" {
			bufferFlushTimerMetric = metric.GetMetric()
		}
	}

	assert.NotNil(t, bufferFlushTimerMetric)

	bufferFlushTimerMetricLabel := bufferFlushTimerMetric[0].GetLabel()[0]
	assert.Equal(t, "listener_id", bufferFlushTimerMetricLabel.GetName())
	assert.Equal(t, "test_buffer", bufferFlushTimerMetricLabel.GetValue())
	// We expect the flush timer to be triggered at least once
	assert.NotEqual(t, float64(0), bufferFlushTimerMetric[0].GetCounter().GetValue())
}
