// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package dogstatsdclienttelemetryimpl implements DogStatsD client telemetry.
package dogstatsdclienttelemetryimpl

import (
	"math"

	dogstatsdclienttelemetry "github.com/DataDog/datadog-agent/comp/aggregator/dogstatsdclienttelemetry/def"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

const (
	dogStatsDClientBytesSentMetric          = "datadog.dogstatsd.client.bytes_sent"
	dogStatsDClientBytesDroppedMetric       = "datadog.dogstatsd.client.bytes_dropped"
	dogStatsDClientBytesDroppedQueueMetric  = "datadog.dogstatsd.client.bytes_dropped_queue"
	dogStatsDClientBytesDroppedWriterMetric = "datadog.dogstatsd.client.bytes_dropped_writer"
)

// Requires defines the dependencies for the DogStatsD client telemetry component.
type Requires struct {
	Telemetry telemetry.Component
}

// Provides defines the values provided by the DogStatsD client telemetry component.
type Provides struct {
	Comp     dogstatsdclienttelemetry.Component
	Observer aggregator.FinalDogStatsDSerieObserver `group:"dogstatsd_final_serie_observers"`
}

type component struct {
	bytesSent          telemetry.SimpleCounter
	bytesDropped       telemetry.SimpleCounter
	bytesDroppedQueue  telemetry.SimpleCounter
	bytesDroppedWriter telemetry.SimpleCounter
}

// NewComponent creates the DogStatsD client telemetry component.
func NewComponent(req Requires) Provides {
	component := &component{
		bytesSent:          req.Telemetry.NewSimpleCounter("dogstatsd_client", "bytes_sent", "Total bytes sent by DogStatsD clients"),
		bytesDropped:       req.Telemetry.NewSimpleCounter("dogstatsd_client", "bytes_dropped", "Total bytes dropped by DogStatsD clients"),
		bytesDroppedQueue:  req.Telemetry.NewSimpleCounter("dogstatsd_client", "bytes_dropped_queue", "Total bytes dropped because the DogStatsD client sender queue is full"),
		bytesDroppedWriter: req.Telemetry.NewSimpleCounter("dogstatsd_client", "bytes_dropped_writer", "Total bytes dropped because the DogStatsD client writer cannot send"),
	}
	return Provides{Comp: component, Observer: component}
}

// ObserveFinalDogStatsDSerie mirrors valid client-byte rate buckets into the
// corresponding internal counter.
func (c *component) ObserveFinalDogStatsDSerie(serie *metrics.Serie) {
	if serie.MType != metrics.APIRateType || serie.Interval <= 0 {
		return
	}

	var counter telemetry.SimpleCounter
	switch serie.Name {
	case dogStatsDClientBytesSentMetric:
		counter = c.bytesSent
	case dogStatsDClientBytesDroppedMetric:
		counter = c.bytesDropped
	case dogStatsDClientBytesDroppedQueueMetric:
		counter = c.bytesDroppedQueue
	case dogStatsDClientBytesDroppedWriterMetric:
		counter = c.bytesDroppedWriter
	default:
		return
	}

	for _, point := range serie.Points {
		bytes := point.Value * float64(serie.Interval)
		if !(bytes >= 0 && bytes < math.MaxUint64) {
			continue
		}
		counter.Add(bytes)
	}
}
