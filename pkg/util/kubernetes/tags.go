// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetes

import "fmt"

// GetStandardTags extracts Unified Service Tagging standard tags (env, service, version)
// from a Kubernetes resource labels map and returns them as formatted "key:value" strings.
// Returns an empty slice if labels is nil or none of the standard labels are present.
func GetStandardTags(labels map[string]string) []string {
	tags := []string{}
	if labels == nil {
		return tags
	}
	labelToTagKey := map[string]string{
		EnvTagLabelKey:     "env",
		ServiceTagLabelKey: "service",
		VersionTagLabelKey: "version",
	}
	for labelKey, tagKey := range labelToTagKey {
		if tagValue, found := labels[labelKey]; found {
			tags = append(tags, fmt.Sprintf("%s:%s", tagKey, tagValue))
		}
	}
	return tags
}
