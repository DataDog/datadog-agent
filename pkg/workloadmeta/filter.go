// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

// Filter allows a subscriber to filter events by entity kind or event source.
type Filter struct {
	kinds   map[Kind]struct{}
	sources map[Source]struct{}
}

// NewFilter creates a new filter for subscribing to workloadmeta events.
func NewFilter(kinds []Kind, sources []Source) *Filter {
	var kindSet map[Kind]struct{}
	if len(kinds) > 0 {
		kindSet = make(map[Kind]struct{})
		for _, k := range kinds {
			kindSet[k] = struct{}{}
		}
	}

	var sourceSet map[Source]struct{}
	if len(sources) > 0 {
		sourceSet = make(map[Source]struct{})
		for _, s := range sources {
			sourceSet[s] = struct{}{}
		}
	}

	return &Filter{
		kinds:   kindSet,
		sources: sourceSet,
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
// filter is nil, or has no sources, it always matches.
func (f *Filter) MatchSource(source Source) bool {
	_, ok := f.SelectSources([]Source{source})

	return ok
}

// SelectSources returns a subset of the passed sources that match the filter.
func (f *Filter) SelectSources(sources []Source) ([]Source, bool) {
	if f == nil || len(f.sources) == 0 {
		return sources, true
	}

	var (
		selectedSources []Source
		found           bool
	)

	for _, s := range sources {
		_, ok := f.sources[s]
		if ok {
			selectedSources = append(selectedSources, s)
			found = true
		}
	}

	return selectedSources, found
}

// Match returns true if the filter matches an event.
func (f *Filter) Match(ev CollectorEvent) bool {
	if f == nil {
		return true
	}

	return f.MatchKind(ev.Entity.GetID().Kind) && f.MatchSource(ev.Source)
}
