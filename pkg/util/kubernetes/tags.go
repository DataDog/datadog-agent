// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetes

// GetStandardTags extracts Unified Service Tagging standard tags (env, service, version)
// from a Kubernetes resource labels map and returns them as formatted "key:value" strings.
// Returns an empty slice if labels is nil or none of the standard labels are present.
func GetStandardTags(labels map[string]string) []string {
	if labels == nil {
		return []string{}
	}
	labelToTagKey := map[string]string{
		EnvTagLabelKey:     "env",
		ServiceTagLabelKey: "service",
		VersionTagLabelKey: "version",
	}
	tags := make([]string, 0, len(labelToTagKey))
	for labelKey, tagKey := range labelToTagKey {
		if tagValue, found := labels[labelKey]; found {
			tags = append(tags, tagKey+":"+tagValue)
		}
	}
	return tags
}
