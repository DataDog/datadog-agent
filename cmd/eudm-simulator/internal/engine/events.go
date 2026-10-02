// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package engine

import (
	"fmt"
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/telemetry"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

// Summarize before Agent serialization can consume or mutate the samples.
// The output worker receives only fixed categories and counts, never payloads.
func describeDelivery(stream schema.Stream, samples []*telemetry.Sample) string {
	count := 0
	var families []string
	for _, sample := range samples {
		switch stream {
		case schema.Metrics:
			count += len(sample.Metrics)
			for _, serie := range sample.Metrics {
				family := telemetrycapture.MetricFamily(serie.Name)
				if family != "" && !slices.Contains(families, family) {
					families = append(families, family)
				}
			}
		case schema.Processes:
			count += len(sample.Processes.Processes)
		case schema.Connections:
			count += len(sample.Connections.Connections)
		case schema.Software:
			count += len(sample.Software.Metadata.Software)
		}
	}
	switch stream {
	case schema.Metrics:
		slices.Sort(families)
		return fmt.Sprintf("%s (%d series)", strings.Join(families, ", "), count)
	case schema.Processes, schema.Connections:
		return fmt.Sprintf("%d %s (%d chunks)", count, stream, len(samples))
	case schema.Software:
		return fmt.Sprintf("inventory (%d applications)", count)
	case schema.HostMetadata:
		return "host metadata"
	default:
		return "snapshot"
	}
}
