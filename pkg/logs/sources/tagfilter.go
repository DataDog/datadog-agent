// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sources

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/comp/logs-library/tagfilter"
)

// TagFilter drops tags that must not leave the Agent. nil means no filtering.
type TagFilter interface {
	// Apply returns the surviving tags. The returned slice must not be modified.
	Apply(tags []string) []string
	// Retains reports whether one tag survives. It allocates nothing.
	Retains(tag string) bool
}

type tagFilterInfo struct {
	global    *tagfilter.Filters
	perSource *tagfilter.Filters
}

func newTagFilterInfo(global, perSource *tagfilter.Filters) *tagFilterInfo {
	return &tagFilterInfo{global: global, perSource: perSource}
}

func (t *tagFilterInfo) InfoKey() string { return "Tag Filters" }

func (t *tagFilterInfo) IsVerbose() bool { return true }

func (t *tagFilterInfo) Info() []string {
	lines := appendTagFilterPatterns(nil, "global", t.global.Patterns())
	return appendTagFilterPatterns(lines, "source", t.perSource.Patterns())
}

func appendTagFilterPatterns(lines []string, scope string, p tagfilter.Patterns) []string {
	if len(p.Include) > 0 {
		lines = append(lines, fmt.Sprintf("%s include: %s", scope, strings.Join(p.Include, ", ")))
	}
	if len(p.Exclude) > 0 {
		lines = append(lines, fmt.Sprintf("%s exclude: %s", scope, strings.Join(p.Exclude, ", ")))
	}
	return lines
}
