// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package dogstatsdclienttelemetryimpl implements DogStatsD client telemetry.
package dogstatsdclienttelemetryimpl

import (
	"math"
	"strings"

	dogstatsdclientdropdetector "github.com/DataDog/datadog-agent/comp/aggregator/dogstatsdclientdropdetector/def"
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
	Telemetry    telemetry.Component
	DropDetector dogstatsdclientdropdetector.Component
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
	dropDetector       dogstatsdclientdropdetector.Component
}

// NewComponent creates the DogStatsD client telemetry component.
func NewComponent(req Requires) Provides {
	tags := []string{"client", "client_transport"}
	component := &component{
		bytesSent:          req.Telemetry.NewCounter("dogstatsd_client", "bytes_sent", tags, "Total bytes sent by DogStatsD clients"),
		bytesDropped:       req.Telemetry.NewCounter("dogstatsd_client", "bytes_dropped", tags, "Total bytes dropped by DogStatsD clients"),
		bytesDroppedQueue:  req.Telemetry.NewCounter("dogstatsd_client", "bytes_dropped_queue", tags, "Total bytes dropped because the DogStatsD client sender queue is full"),
		bytesDroppedWriter: req.Telemetry.NewCounter("dogstatsd_client", "bytes_dropped_writer", tags, "Total bytes dropped because the DogStatsD client writer cannot send"),
		dropDetector:       req.DropDetector,
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
	var metric dogstatsdclientdropdetector.ClientByteMetric
	switch serie.Name {
	case dogStatsDClientBytesSentMetric:
		counter = c.bytesSent
		metric = dogstatsdclientdropdetector.ClientByteMetricSent
	case dogStatsDClientBytesDroppedMetric:
		counter = c.bytesDropped
		metric = dogstatsdclientdropdetector.ClientByteMetricDropped
	case dogStatsDClientBytesDroppedQueueMetric:
		counter = c.bytesDroppedQueue
		metric = dogstatsdclientdropdetector.ClientByteMetricDroppedQueue
	case dogStatsDClientBytesDroppedWriterMetric:
		counter = c.bytesDroppedWriter
		metric = dogstatsdclientdropdetector.ClientByteMetricDroppedWriter
	default:
		return
	}

	client, transport := clientTelemetryTags(serie)
	var totalBytes float64
	for _, point := range serie.Points {
		bytes := point.Value * float64(serie.Interval)
		if !(bytes >= 0 && bytes < math.MaxUint64) {
			continue
		}
		counter.Add(bytes, client, transport)
		totalBytes += bytes
	}

	if totalBytes > 0 && (transport == "uds" || transport == "uds-stream") {
		c.dropDetector.ObserveClientBytes(metric, totalBytes)
	}
}

func clientTelemetryTags(serie *metrics.Serie) (string, string) {
	client := unknownTagValue
	transport := unknownTagValue
	serie.Tags.Find(func(tag string) bool {
		if strings.HasPrefix(tag, dogStatsDClientLibraryTagPrefix) {
			client = normalizeClientLibrary(strings.TrimPrefix(tag, dogStatsDClientLibraryTagPrefix))
		} else if strings.HasPrefix(tag, dogStatsDClientTransportTagPrefix) {
			transport = normalizeClientTransport(strings.TrimPrefix(tag, dogStatsDClientTransportTagPrefix))
		}
		return client != unknownTagValue && transport != unknownTagValue
	})
	return client, transport
}

func normalizeClientLibrary(client string) string {
	switch client {
	case "go", "py", "java", "ruby", "csharp", "php", "rust":
		return client
	default:
		return unknownTagValue
	}
}

func normalizeClientTransport(transport string) string {
	switch transport {
	case "udp", "uds", "uds-stream", "uds-datagram", "pipe", "namedpipe", "named_pipe", "custom", "http":
		return transport
	default:
		return unknownTagValue
	}
}
