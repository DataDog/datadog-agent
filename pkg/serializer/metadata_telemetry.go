// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serializer

import (
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/DataDog/datadog-agent/pkg/serializer/split"
)

// Values of the "encoding" tag of the dropped metadata payload size histogram.
const (
	encodingCompressed   = "compressed"
	encodingUncompressed = "uncompressed"
)

// metadataDropMetrics holds the telemetry of the metadata payloads dropped by the serializer for exceeding the
// payload size limits. Metadata payloads cannot be split, so those payloads are never sent and this telemetry is the
// only way to see them. Both metrics are part of the cross-org agent telemetry, see the "logs-and-metrics" profile in
// comp/core/agenttelemetry/impl/defaultProfiles.yaml.
type metadataDropMetrics struct {
	payloads telemetry.Counter
	bytes    telemetry.Histogram
}

func newMetadataDropMetrics(tel telemetry.Component) *metadataDropMetrics {
	return &metadataDropMetrics{
		payloads: tel.NewCounter("serializer", "metadata_payload_dropped",
			[]string{"reason"},
			"Metadata payloads dropped because they exceed the payload size limits, by limit exceeded"),
		bytes: tel.NewHistogram("serializer", "metadata_payload_dropped_bytes",
			[]string{"encoding"},
			"Size, in bytes, of the metadata payloads dropped because they exceed the payload size limits",
			[]float64{2 << 20, 3 << 20, 4 << 20, 8 << 20, 16 << 20, 32 << 20, 64 << 20, 128 << 20, 256 << 20}),
	}
}

var defaultMetadataDropMetrics = newMetadataDropMetrics(telemetryimpl.GetCompatComponent())

// record counts a dropped metadata payload and observes both its compressed and uncompressed sizes.
func (m *metadataDropMetrics) record(exceeded split.SizeLimit, compressedSize, uncompressedSize int) {
	m.payloads.Inc(string(exceeded))
	m.bytes.Observe(float64(compressedSize), encodingCompressed)
	m.bytes.Observe(float64(uncompressedSize), encodingUncompressed)
}
