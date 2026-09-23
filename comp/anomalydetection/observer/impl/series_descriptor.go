// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"sort"
	"strings"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

// formatSeriesDescriptor returns the legacy stable representation used at
// debug, UI, and digest boundaries: "namespace|name:agg|host|tag1,tag2,...".
// It must not be used as runtime identity for storage-backed series.
func formatSeriesDescriptor(sd observerdef.SeriesDescriptor) string {
	tags := sortedSeriesDescriptorTags(sd.Tags)
	return sd.Namespace + "|" + sd.Name + ":" + observerdef.AggregateString(sd.Aggregate) + "|" + sd.Host + "|" + strings.Join(tags, ",")
}

func sortedSeriesDescriptorTags(tags []string) []string {
	sorted := append([]string(nil), tags...)
	sort.Strings(sorted)
	return sorted
}
