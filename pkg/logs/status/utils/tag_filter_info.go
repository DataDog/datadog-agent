// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import "strings"

// TagFilterInfo renders a source's effective tag filters for the verbose status page.
type TagFilterInfo struct {
	globalInclude []string
	globalExclude []string
	sourceInclude []string
	sourceExclude []string
}

// NewTagFilterInfo returns a verbose-only status provider describing a source's effective tag filters.
func NewTagFilterInfo(globalInclude, globalExclude, sourceInclude, sourceExclude []string) *TagFilterInfo {
	return &TagFilterInfo{
		globalInclude: globalInclude,
		globalExclude: globalExclude,
		sourceInclude: sourceInclude,
		sourceExclude: sourceExclude,
	}
}

// InfoKey returns the key for this info provider.
func (t *TagFilterInfo) InfoKey() string {
	return "Tag Filters"
}

// IsVerbose reports that tag filters only render on the verbose status page.
func (t *TagFilterInfo) IsVerbose() bool {
	return true
}

// Info returns one line per non-empty filter scope, omitting scopes with no patterns.
func (t *TagFilterInfo) Info() []string {
	var info []string
	appendScope := func(label string, patterns []string) {
		if len(patterns) == 0 {
			return
		}
		info = append(info, label+strings.Join(patterns, ", "))
	}

	appendScope("global include: ", t.globalInclude)
	appendScope("global exclude: ", t.globalExclude)
	appendScope("source include: ", t.sourceInclude)
	appendScope("source exclude: ", t.sourceExclude)

	return info
}
