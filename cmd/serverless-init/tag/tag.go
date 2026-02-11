// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(SERV) Fix revive linter
package tag

import (
	"maps"
	"os"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/serverless/tags"
)

const (
	// enableBackendTraceStatsEnvVar is the environment variable to enable backend trace stats computation
	// This computation is known to be incorrect since it does not account for traces that get
	// sampled out in the agent and don't get sent to the backend.
	enableBackendTraceStatsEnvVar = "DD_SERVERLESS_INIT_ENABLE_BACKEND_TRACE_STATS"
)

// highCardinalityTags is a set of tag keys that should be excluded from metrics
var highCardinalityTags = map[string]struct{}{
	"container_id":        {},
	"gcr.container_id":    {},
	"gcrfx.container_id":  {},
	"replica_name":        {},
	"aca.replica.name":    {},
	"gcrj.execution_name": {},
	"gcrj.task_index":     {},
	"gcrj.task_attempt":   {},
	"gcrj.task_count":     {},
}

// TagPair contains a pair of tag key and value
//
//nolint:revive // TODO(SERV) Fix revive linter
type TagPair struct {
	name    string
	envName string
}

func getTagFromEnv(envName string) (string, bool) {
	value := os.Getenv(envName)
	if len(value) == 0 {
		return "", false
	}
	return strings.ToLower(value), true
}

// GetBaseTagsMapWithMetadata returns a map of the three reserved Unified Service Tagging tags, to be used everywhere
func GetBaseTagsMapWithMetadata(metadata map[string]string, versionMode string) map[string]string {
	tagsMap := map[string]string{}
	listTags := []TagPair{
		{
			name:    "env",
			envName: "DD_ENV",
		},
		{
			name:    "service",
			envName: "DD_SERVICE",
		},
		{
			name:    "version",
			envName: "DD_VERSION",
		},
	}
	for _, tagPair := range listTags {
		if value, found := getTagFromEnv(tagPair.envName); found {
			tagsMap[tagPair.name] = value
		}
	}

	maps.Copy(tagsMap, metadata)

	tagsMap[versionMode] = tags.GetExtensionVersion()
	return tagsMap
}

// MakeTraceAgentTags handles tag customization for the trace agent.
//
// * Adds tags needed for accurate trace metric stats computation to a new tag map.
// Some traces are sampled out in the agent and don't get sent to the backend.
// If "_dd.compute_stats" is enabled, we make sure to count the unsampled traces when computing trace stat metrics.
// If "_dd.compute_stats" is disabled, the result is known incorrect data.
func MakeTraceAgentTags(tagsMap map[string]string) map[string]string {
	if enabled, _ := strconv.ParseBool(os.Getenv(enableBackendTraceStatsEnvVar)); enabled {
		// Use of clone instead of copy creates a new map to avoid polluting other agent components.
		newTags := maps.Clone(tagsMap)
		newTags[tags.ComputeStatsKey] = tags.ComputeStatsValue
		return newTags
	}
	return tagsMap
}

// MakeMetricAgentTags returns two tag maps for the metric agent:
//   - baseTags: tags with high cardinality keys excluded for use with dogstatsd metrics and enhanced metrics that don't need high cardinality tags..
//   - highCardinalityTags: tags with only the high cardinality keys for use with enhanced metrics that need high cardinality tags.
//
// We avoid high-cardinality tags on base metrics by default due to cost, as we store and bill by cardinality.
func MakeMetricAgentTags(tags map[string]string) (baseTags map[string]string, highCardinalityTags map[string]string) {
	baseTags = make(map[string]string, len(tags))
	highCardinalityTags = make(map[string]string)
	for k, v := range tags {
		if _, isHighCardinality := highCardinalityTags[k]; isHighCardinality {
			highCardinalityTags[k] = v
		} else {
			baseTags[k] = v
		}
	}
	return baseTags, highCardinalityTags
}
