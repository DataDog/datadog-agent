// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package types

import (
	"maps"
)

// FilterBuilder builds a tagger subscriber filter based on include/exclude rules
type FilterBuilder struct {
	prefixesToInclude map[EntityIDPrefix]struct{}

	prefixesToExclude map[EntityIDPrefix]struct{}
}

// NewFilterBuilder returns a new empty filter builder
func NewFilterBuilder() *FilterBuilder {
	return &FilterBuilder{
		prefixesToInclude: make(map[EntityIDPrefix]struct{}),
		prefixesToExclude: make(map[EntityIDPrefix]struct{}),
	}
}

// Include includes the specified prefixes in the filter
func (fb *FilterBuilder) Include(prefixes ...EntityIDPrefix) *FilterBuilder {
	if fb == nil {
		panic("filter builder should not be nil")
	}

	for _, prefix := range prefixes {
		fb.prefixesToInclude[prefix] = struct{}{}
	}

	return fb
}

// Exclude excludes the specified prefixes from the filter
func (fb *FilterBuilder) Exclude(prefixes ...EntityIDPrefix) *FilterBuilder {
	if fb == nil {
		panic("filter builder should not be nil")
	}

	for _, prefix := range prefixes {
		fb.prefixesToExclude[prefix] = struct{}{}
	}

	return fb
}

// Build builds a new Filter object based on the calls to Include and Exclude
// If the builder only excludes prefixes, the created filter will match any prefix except for the excluded ones.
// If the builder only includes prefixes, the created filter will match only the prefixes included in the builder.
// If the builder includes prefixes and excludes prefixes, the created filter will match only prefixes that are included but a not excluded in the builder
// If the builder has neither included nor excluded prefixes, it will match by default all prefixes among `AllPrefixesSet` prefixes
func (fb *FilterBuilder) Build(card TagCardinality) *Filter {
	if fb == nil {
		panic("filter builder should not be nil")
	}

	if len(fb.prefixesToInclude)+len(fb.prefixesToExclude) == 0 {
		return newFilter(AllPrefixesSet(), card)
	}

	var prefixSet map[EntityIDPrefix]struct{}

	// initialise prefixSet with what should be included
	if len(fb.prefixesToInclude) == 0 {
		prefixSet = maps.Clone(AllPrefixesSet())
	} else {
		prefixSet = maps.Clone(fb.prefixesToInclude)
	}

	// exclude unwanted prefixes
	for prefix := range fb.prefixesToExclude {
		delete(prefixSet, prefix)
	}

	return newFilter(prefixSet, card)
}
