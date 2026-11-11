// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sources

// TagFilter removes tags before intake encoding. It lives here to avoid an import
// cycle; implementations must be immutable and must not mutate Keep's input.
type TagFilter interface {
	// Keep returns the tags that survive the filter, preserving order. It may
	// return the input slice itself when nothing is removed.
	Keep(tags []string) []string

	// RetainsTag reports whether a tag with the provided key and value survives.
	RetainsTag(key, value string) bool
}
