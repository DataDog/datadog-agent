// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(SERV) Fix revive linter
package tags

import (
	"fmt"
	"maps"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// ComputeStatsKey is the tag key indicating whether trace stats should be computed
	ComputeStatsKey = "_dd.compute_stats"
	// ComputeStatsValue is the tag value indicating trace stats should be computed
	ComputeStatsValue = "1"
)

// currentExtensionVersion represents the current version of the Datadog Lambda Extension.
// It is applied to all telemetry as a tag.
// It is replaced at build time with an actual version number by build scripts in the datadog-lambda-extension repo.
var currentExtensionVersion = "xxx"

//nolint:revive // TODO(SERV) Fix revive linter
func ArrayToMap(tagArray []string) map[string]string {
	tagMap := make(map[string]string)
	for _, tag := range tagArray {
		splitTags := strings.SplitSeq(tag, ",")
		for singleTag := range splitTags {
			tagMap = addTag(tagMap, singleTag)
		}
	}
	return tagMap
}

func MapToArray(tagsMap map[string]string) []string {
	tagsArray := make([]string, 0, len(tagsMap))
	for key, value := range tagsMap {
		tagsArray = append(tagsArray, fmt.Sprintf("%s:%s", key, value))
	}
	return tagsArray
}

func MergeWithOverwrite(tags map[string]string, overwritingTags map[string]string) map[string]string {
	merged := make(map[string]string)
	maps.Copy(merged, tags)
	maps.Copy(merged, overwritingTags)
	return merged
}

// GetExtensionVersion returns the extension version which is fed at build time
func GetExtensionVersion() string {
	return currentExtensionVersion
}

func addTag(tagMap map[string]string, tag string) map[string]string {
	extract := strings.SplitN(tag, ":", 2)
	if len(extract) == 2 {
		tagMap[strings.ToLower(extract[0])] = strings.ToLower(extract[1])
	} else {
		log.Warn("Tag" + tag + " has not expected format")
	}
	return tagMap
}
