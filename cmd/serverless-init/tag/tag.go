// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tag

import (
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/serverless/tags"
)

// TagPair contains a pair of tag key and value
type TagPair struct {
	name    string
	envName string
}

func getTag(envName string) (string, bool) {
	value := os.Getenv(envName)
	if len(value) == 0 {
		return "", false
	}
	return strings.ToLower(value), true
}

// GetBaseTagsMapWithMetadata returns a map of Datadog's base tags
func GetBaseTagsMapWithMetadata(metadata map[string]string) map[string]string {
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
		if value, found := getTag(tagPair.envName); found {
			tagsMap[tagPair.name] = value
		}
	}

	for key, value := range metadata {
		tagsMap[key] = value
	}

	tagsMap["datadog_init_version"] = tags.GetExtensionVersion()

	return tagsMap
}

// GetBaseTagsArrayWithMetadataTags see GetBaseTagsMapWithMetadata (as array)
func GetBaseTagsArrayWithMetadataTags(metadata map[string]string) []string {
	tagsMap := GetBaseTagsMapWithMetadata(metadata)
	return tags.MapToArray(tagsMap)
}
