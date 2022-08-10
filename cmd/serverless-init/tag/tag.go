// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tag

import (
	"fmt"
	"os"
	"strings"
)

type tagPair struct {
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

// GetBaseTagsMapWithMetadata returns a map of Datadog's base tags + Cloud Run specific if present
func GetBaseTagsMapWithMetadata(metadata map[string]string) map[string]string {
	tags := map[string]string{}
	listTags := []tagPair{
		{
			name:    "revision_name",
			envName: "K_REVISION",
		},
		{
			name:    "service_name",
			envName: "K_SERVICE",
		},
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
			tags[tagPair.name] = value
		}
	}

	for key, value := range metadata {
		tags[key] = value
	}

	tags["origin"] = "cloudrun"

	return tags
}

// GetBaseTagsArrayWithMetadataTags see GetBaseTagsMapWithMetadata (as array)
func GetBaseTagsArrayWithMetadataTags(metadata map[string]string) []string {
	tagsMap := GetBaseTagsMapWithMetadata(metadata)
	tagsArray := make([]string, 0, len(tagsMap))
	for key, value := range tagsMap {
		tagsArray = append(tagsArray, fmt.Sprintf("%s:%s", key, value))
	}
	return tagsArray
}
