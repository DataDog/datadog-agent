// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package metrics provides metric creation helpers for the dogtelextension.
package metrics

import (
	"go.opentelemetry.io/collector/component"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// TagsFromBuildInfo returns a list of tags derived from buildInfo to be used when creating metrics.
func TagsFromBuildInfo(buildInfo component.BuildInfo) []string {
	var tags []string
	if buildInfo.Version != "" {
		tags = append(tags, "version:"+buildInfo.Version)
	}
	if buildInfo.Command != "" {
		tags = append(tags, "command:"+buildInfo.Command)
	}
	return tags
}

// CreateLivenessSerie creates a liveness metric serie to report that the dogtel extension is running.
// The timestamp should be in Unix nanoseconds.
func CreateLivenessSerie(hostname string, timestampNs uint64, tags []string) *metrics.Serie {
	// Transform UnixNano timestamp into Unix timestamp (seconds)
	timestamp := float64(timestampNs / 1e9)

	return &metrics.Serie{
		Name:           "otel.dogtel_extension.running",
		Points:         []metrics.Point{{Ts: timestamp, Value: 1.0}},
		Tags:           tagset.NewCompositeTags(tags, nil),
		Host:           hostname,
		MType:          metrics.APIGaugeType,
		SourceTypeName: "otel.dogtel_extension",
		Source:         metrics.MetricSourceOpenTelemetryCollectorUnknown,
	}
}
