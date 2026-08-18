// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package networkpath

// mergeTags returns the stable union of the supplied tag sets. Tags are
// deduplicated by their complete string, so distinct values for the same tag
// key are preserved.
func mergeTags(tagSets ...[]string) []string {
	seen := make(map[string]struct{})
	var merged []string
	for _, tags := range tagSets {
		for _, tag := range tags {
			if _, found := seen[tag]; found {
				continue
			}
			seen[tag] = struct{}{}
			merged = append(merged, tag)
		}
	}
	return merged
}
