// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(SERV) Fix revive linter
package tag

import (
	"maps"
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/serverless/tags"
)

const (
	// enableBackendTraceStatsEnvVar is the environment variable to enable backend trace stats computation
	// This computation is known to be incorrect since it does not account for traces that get
	// sampled out in the agent and don't get sent to the backend.
	enableBackendTraceStatsEnvVar = "DD_SERVERLESS_INIT_ENABLE_BACKEND_TRACE_STATS"
)

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

// GetBaseTagsMapWithMetadata returns a map of Datadog's base tags
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
	// Only set compute_stats tag if explicitly enabled. This is known to be
	// incorrect since it does not include traces that get sampled out in the
	// agent and don't get sent to the backend.
	if os.Getenv(enableBackendTraceStatsEnvVar) == "true" {
		tagsMap[tags.ComputeStatsKey] = tags.ComputeStatsValue
	}

	return tagsMap
}

// WithoutHihCardinalityTags creates a new tag map without high cardinality tags we use on traces
func WithoutHighCardinalityTags(tags map[string]string) map[string]string {
	newTags := make(map[string]string, len(tags))
	for k, v := range tags {
		if k != "container_id" &&
			k != "gcr.container_id" &&
			k != "gcrfx.container_id" &&
			k != "replica_name" &&
			k != "aca.replica.name" {
			newTags[k] = v
		}
	}
	return newTags
}
