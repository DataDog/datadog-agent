// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

// Filter allows a subscriber to filter events by entity kind or event source.
type Filter struct {
	kinds  map[Kind]struct{}
	source Source
}

// NewFilter creates a new filter for subscribing to workloadmeta events.
func NewFilter(kinds []Kind, source Source) *Filter {
	var kindSet map[Kind]struct{}
	if len(kinds) > 0 {
		kindSet = make(map[Kind]struct{})
		for _, k := range kinds {
			kindSet[k] = struct{}{}
		}
	}

	return &Filter{
		kinds:  kindSet,
		source: source,
	}
}

// MatchKind returns true if the filter matches the passed Kind. If the filter
// is nil, or has no kinds, it always matches.
func (f *Filter) MatchKind(k Kind) bool {
	if f == nil || len(f.kinds) == 0 {
		return true
	}

	_, ok := f.kinds[k]

	return ok
}

// MatchSource returns true if the filter matches the passed sources. If the
// filter is nil, or has SourceAll, it always matches.
func (f *Filter) MatchSource(source Source) bool {
	if source == SourceAll || f.Source() == SourceAll {
		return true
	}

	return f.Source() == source
}

// Source returns the source this filter is filtering by. If the filter is nil,
// returns SourceAll.
func (f *Filter) Source() Source {
	if f == nil {
		return SourceAll
	}

	return f.source
}

// Match returns true if the filter matches an event.
func (f *Filter) Match(ev CollectorEvent) bool {
	if f == nil {
		return true
	}

	return f.MatchKind(ev.Entity.GetID().Kind) && f.MatchSource(ev.Source)
}
