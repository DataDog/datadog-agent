// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package types

import (
	"maps"
)

// NewMatchAllFilter returns a filter that matches any prefix
func NewMatchAllFilter() *Filter {
	return nil
}

// Filter represents a subscription filter for the tagger
type Filter struct {
	prefixes    map[EntityIDPrefix]struct{}
	cardinality TagCardinality
}

func newFilter(prefixes map[EntityIDPrefix]struct{}, cardinality TagCardinality) *Filter {
	return &Filter{
		prefixes:    maps.Clone(prefixes),
		cardinality: cardinality,
	}
}

// GetPrefixes returns the prefix set of the filter
// If the filter is nil, a set containing all possible prefixes is returned
func (f *Filter) GetPrefixes() map[EntityIDPrefix]struct{} {
	if f == nil {
		return AllPrefixesSet()
	}

	return maps.Clone(f.prefixes)
}

// GetCardinality returns the filter cardinality
// If the filter is nil, High cardinality is returned
func (f *Filter) GetCardinality() TagCardinality {
	if f == nil {
		return HighCardinality
	}

	return f.cardinality
}

// MatchesPrefix returns whether or not the filter matches the prefix passed as argument
func (f *Filter) MatchesPrefix(prefix EntityIDPrefix) bool {
	// A nil filter should match everything
	if f == nil {
		return true
	}

	_, found := f.prefixes[prefix]

	return found
}
