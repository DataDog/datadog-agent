// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sources

// TagFilter removes tags from a log's serialized tag set before it is encoded for
// the intake.
//
// The interface is declared here rather than alongside its implementation so that
// neither this package nor pkg/logs/message needs to depend on comp/logs-library.
// Implementations must be immutable, safe for concurrent use, nil-receiver safe,
// and must never mutate the slice passed to Keep.
type TagFilter interface {
	// Keep returns the tags that survive the filter, preserving order. It may
	// return the input slice itself when nothing is removed.
	Keep(tags []string) []string

	// Retains reports whether a single "key:value" tag survives the filter.
	Retains(tag string) bool
}
