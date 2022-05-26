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

func GetBaseTagsMap() map[string]string {
	tags := map[string]string{}
	listTags := []tagPair{
		{
			name:    "cloudrunrevision",
			envName: "K_REVISION",
		},
		{
			name:    "cloudrunservice",
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
	return tags
}

func GetBaseTagsArray() []string {
	tagsMap := GetBaseTagsMap()
	tagsArray := make([]string, 0, len(tagsMap))
	for key, value := range tagsMap {
		tagsArray = append(tagsArray, fmt.Sprintf("%s:%s", key, value))
	}
	return tagsArray
}
