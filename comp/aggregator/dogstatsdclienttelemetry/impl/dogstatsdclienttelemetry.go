// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package dogstatsdclienttelemetryimpl implements DogStatsD client telemetry.
package dogstatsdclienttelemetryimpl

import (
	"math"
	"strings"

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
	dogStatsDClientLibraryTagPrefix         = "client:"
	dogStatsDClientTransportTagPrefix       = "client_transport:"
	unknownTagValue                         = "unknown"
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
	bytesSent          telemetry.Counter
	bytesDropped       telemetry.Counter
	bytesDroppedQueue  telemetry.Counter
	bytesDroppedWriter telemetry.Counter
}

// NewComponent creates the DogStatsD client telemetry component.
func NewComponent(req Requires) Provides {
	tags := []string{"client", "client_transport"}
	component := &component{
		bytesSent:          req.Telemetry.NewCounter("dogstatsd_client", "bytes_sent", tags, "Total bytes sent by DogStatsD clients"),
		bytesDropped:       req.Telemetry.NewCounter("dogstatsd_client", "bytes_dropped", tags, "Total bytes dropped by DogStatsD clients"),
		bytesDroppedQueue:  req.Telemetry.NewCounter("dogstatsd_client", "bytes_dropped_queue", tags, "Total bytes dropped because the DogStatsD client sender queue is full"),
		bytesDroppedWriter: req.Telemetry.NewCounter("dogstatsd_client", "bytes_dropped_writer", tags, "Total bytes dropped because the DogStatsD client writer cannot send"),
	}
	return Provides{Comp: component, Observer: component}
}

// ObserveFinalDogStatsDSerie mirrors valid client-byte rate buckets into the
// corresponding internal counter.
func (c *component) ObserveFinalDogStatsDSerie(serie *metrics.Serie) {
	if serie.MType != metrics.APIRateType || serie.Interval <= 0 {
		return
	}

	var counter telemetry.Counter
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

	client, transport := clientTelemetryTags(serie)
	for _, point := range serie.Points {
		bytes := point.Value * float64(serie.Interval)
		if !(bytes >= 0 && bytes < math.MaxUint64) {
			continue
		}
		counter.Add(bytes, client, transport)
	}
}

func clientTelemetryTags(serie *metrics.Serie) (string, string) {
	client := unknownTagValue
	transport := unknownTagValue
	serie.Tags.Find(func(tag string) bool {
		if strings.HasPrefix(tag, dogStatsDClientLibraryTagPrefix) {
			client = strings.TrimPrefix(tag, dogStatsDClientLibraryTagPrefix)
		} else if strings.HasPrefix(tag, dogStatsDClientTransportTagPrefix) {
			transport = strings.TrimPrefix(tag, dogStatsDClientTransportTagPrefix)
		}
		return client != unknownTagValue && transport != unknownTagValue
	})
	return client, transport
}
