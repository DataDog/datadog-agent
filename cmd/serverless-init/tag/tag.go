// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(SERV) Fix revive linter
package tag

import (
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/serverless/tags"
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

	for key, value := range metadata {
		tagsMap[key] = value
	}

	tagsMap[versionMode] = tags.GetExtensionVersion()
	tagsMap[tags.ComputeStatsKey] = tags.ComputeStatsValue

	return tagsMap
}

// WithoutContainerID creates a new tag map without the `container_id` tag
func WithoutContainerID(tags map[string]string) map[string]string {
	newTags := make(map[string]string, len(tags))
	for k, v := range tags {
		if k != "container_id" {
			newTags[k] = v
		}
	}
	return newTags
}
