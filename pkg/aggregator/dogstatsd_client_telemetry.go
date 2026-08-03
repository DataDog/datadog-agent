// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package aggregator

import (
	"math"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

const (
	dogStatsDClientBytesSentMetric          = "datadog.dogstatsd.client.bytes_sent"
	dogStatsDClientBytesDroppedMetric       = "datadog.dogstatsd.client.bytes_dropped"
	dogStatsDClientBytesDroppedQueueMetric  = "datadog.dogstatsd.client.bytes_dropped_queue"
	dogStatsDClientBytesDroppedWriterMetric = "datadog.dogstatsd.client.bytes_dropped_writer"
)

var dogStatsDClientTelemetryCollector = newDogStatsDClientTelemetry(
	telemetryimpl.GetCompatComponent().NewSimpleCounter("dogstatsd_client", "bytes_sent", "Total bytes sent by DogStatsD clients"),
	telemetryimpl.GetCompatComponent().NewSimpleCounter("dogstatsd_client", "bytes_dropped", "Total bytes dropped by DogStatsD clients"),
	telemetryimpl.GetCompatComponent().NewSimpleCounter("dogstatsd_client", "bytes_dropped_queue", "Total bytes dropped because the DogStatsD client sender queue is full"),
	telemetryimpl.GetCompatComponent().NewSimpleCounter("dogstatsd_client", "bytes_dropped_writer", "Total bytes dropped because the DogStatsD client writer cannot send"),
)

type dogStatsDClientTelemetry struct {
	bytesSent          telemetry.SimpleCounter
	bytesDropped       telemetry.SimpleCounter
	bytesDroppedQueue  telemetry.SimpleCounter
	bytesDroppedWriter telemetry.SimpleCounter
}

func newDogStatsDClientTelemetry(
	bytesSent telemetry.SimpleCounter,
	bytesDropped telemetry.SimpleCounter,
	bytesDroppedQueue telemetry.SimpleCounter,
	bytesDroppedWriter telemetry.SimpleCounter,
) *dogStatsDClientTelemetry {
	return &dogStatsDClientTelemetry{
		bytesSent:          bytesSent,
		bytesDropped:       bytesDropped,
		bytesDroppedQueue:  bytesDroppedQueue,
		bytesDroppedWriter: bytesDroppedWriter,
	}
}

// observe mirrors valid post-aggregation client-byte rate buckets into the corresponding internal counter.
func (t *dogStatsDClientTelemetry) observe(serie *metrics.Serie) {
	if serie.MType != metrics.APIRateType || serie.Interval <= 0 {
		return
	}

	var counter telemetry.SimpleCounter
	switch serie.Name {
	case dogStatsDClientBytesSentMetric:
		counter = t.bytesSent
	case dogStatsDClientBytesDroppedMetric:
		counter = t.bytesDropped
	case dogStatsDClientBytesDroppedQueueMetric:
		counter = t.bytesDroppedQueue
	case dogStatsDClientBytesDroppedWriterMetric:
		counter = t.bytesDroppedWriter
	default:
		return
	}

	for _, point := range serie.Points {
		bytes := point.Value * float64(serie.Interval)
		if !bytesAreValidCounterDelta(bytes) {
			continue
		}
		counter.Add(bytes)
	}
}

func bytesAreValidCounterDelta(bytes float64) bool {
	return bytes >= 0 &&
		!math.IsNaN(bytes) &&
		!math.IsInf(bytes, 0) &&
		bytes == math.Trunc(bytes) &&
		bytes < float64(^uint64(0))
}
