// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	filterlist "github.com/DataDog/datadog-agent/comp/filterlist/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/tags"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// Check samplers track every source of a metric with its full tags, so that state kept per source stays per source,
// and filter the tags that metric_tag_filterlist rules remove only from their output. Each context references the
// identity it filters to, which the contexts of every check sampler that filter to it share.

// tagFilterFlags record what check samplers do with a context's output.
type tagFilterFlags uint8

const (
	// hasFilterableType is set once the context gets samples of a type whose tags can be filtered at output.
	hasFilterableType tagFilterFlags = 1 << iota
	// hasUnfilterableType is set once the context gets samples of another type. Context keys don't include the
	// metric type, so samples of several types can share a context: then none of its output is filtered, from then
	// on, and a point the merger holds for it can be sent again for the same second.
	hasUnfilterableType
	// routedToMerger is set on the contexts of metrics filtered since the Agent started: their output goes to the
	// check output merger, filtered while a rule applies and with its full tags otherwise.
	routedToMerger
)

func (f *tagFilterFlags) set(flag tagFilterFlags) {
	*f |= flag
}

// filtersAtOutput returns whether check samplers filter the context's output when a rule matches it.
func (c *Context) filtersAtOutput() bool {
	return c.tagFilterFlags&(hasFilterableType|hasUnfilterableType) == hasFilterableType
}

// mergesAtOutput returns whether the context's output goes to the check output merger: its metric has been
// filtered since the Agent started.
func (c *Context) mergesAtOutput() bool {
	return c.tagFilterFlags&routedToMerger != 0 && c.filtersAtOutput()
}

// typeFiltersAtOutput returns whether check samplers can filter the tags of a metric type from their output: the types
// whose sources' points add up.
func typeFiltersAtOutput(mtype metrics.MetricType) bool {
	switch mtype {
	case metrics.DistributionType, metrics.CounterType:
		return true
	}
	return false
}

// filteredIdentity is the identity of a context after metric_tag_filterlist rules: its key and the tags left
// after filtering, with the same name and host.
type filteredIdentity struct {
	key        ckey.ContextKey
	taggerTags *tags.Entry
	metricTags *tags.Entry
	refs       uint32
}

// Tags returns the tags left after filtering.
func (s *filteredIdentity) Tags() tagset.CompositeTags {
	return tagset.NewCompositeTags(s.taggerTags.Tags(), s.metricTags.Tags())
}

// filteredIdentityTable holds one filteredIdentity per filtered series, referenced by 1 + its index, and the names
// of the metrics filtered since the Agent started. The check samplers share one, as they are all used from the
// aggregator's run loop.
type filteredIdentityTable struct {
	identities       []filteredIdentity
	free             []uint32
	byKey            map[ckey.ContextKey]uint32
	filteredNames    map[string]struct{}
	pendingNotBefore map[string]int64
	lastCommitSecond int64
}

func newFilteredIdentityTable() *filteredIdentityTable {
	return &filteredIdentityTable{
		byKey:            make(map[ckey.ContextKey]uint32),
		filteredNames:    make(map[string]struct{}),
		pendingNotBefore: make(map[string]int64),
	}
}

// noteCommit records the time a check sampler committed at.
func (t *filteredIdentityTable) noteCommit(timestamp float64) {
	t.lastCommitSecond = int64(timestamp)
}

// notePeriodicFlush records the time a periodic flush stamped series at. It's a second in the past, so it only moves
// the record forward: a commit of another check may have used a later second.
func (t *filteredIdentityTable) notePeriodicFlush(timestamp float64) {
	t.lastCommitSecond = max(t.lastCommitSecond, int64(timestamp))
}

// markFiltered records that a metric is filtered. The first time, its merged points can't use the seconds up to the
// latest commit, which its output sent before may have used.
func (t *filteredIdentityTable) markFiltered(name string) {
	if _, ok := t.filteredNames[name]; ok {
		return
	}
	t.filteredNames[name] = struct{}{}
	t.pendingNotBefore[name] = t.lastCommitSecond + 1
}

// takeNotBefore returns, for the metrics first filtered since the last call, the first second their merged points
// can use.
func (t *filteredIdentityTable) takeNotBefore() map[string]int64 {
	if len(t.pendingNotBefore) == 0 {
		return nil
	}
	notBefore := t.pendingNotBefore
	t.pendingNotBefore = make(map[string]int64)
	return notBefore
}

// wasFiltered returns whether a metric has been filtered since the Agent started.
func (t *filteredIdentityTable) wasFiltered(name string) bool {
	_, ok := t.filteredNames[name]
	return ok
}

// acquire returns a reference to the identity with the key, creating it with newTags if there is none.
func (t *filteredIdentityTable) acquire(key ckey.ContextKey, newTags func() (taggerTags, metricTags *tags.Entry)) uint32 {
	if ref, ok := t.byKey[key]; ok {
		t.identities[ref-1].refs++
		return ref
	}
	identity := filteredIdentity{key: key, refs: 1}
	identity.taggerTags, identity.metricTags = newTags()
	var ref uint32
	if n := len(t.free); n > 0 {
		ref = t.free[n-1]
		t.free = t.free[:n-1]
		t.identities[ref-1] = identity
	} else {
		t.identities = append(t.identities, identity)
		ref = uint32(len(t.identities))
	}
	t.byKey[key] = ref
	return ref
}

// release drops a reference, removing the identity with its last one.
func (t *filteredIdentityTable) release(ref uint32) {
	identity := &t.identities[ref-1]
	if identity.refs--; identity.refs > 0 {
		return
	}
	identity.taggerTags.Release()
	identity.metricTags.Release()
	delete(t.byKey, identity.key)
	*identity = filteredIdentity{}
	t.free = append(t.free, ref)
}

// get returns the identity for a reference. It is valid until the next acquire.
func (t *filteredIdentityTable) get(ref uint32) *filteredIdentity {
	return &t.identities[ref-1]
}

// trackOutput records what a sample adds to its context's output. The first sample of a type that can be filtered
// filters the context if a rule matches: the identity only depends on the context, so the rule is looked up once
// (ShouldStripTags allocates), and refilter updates contexts when rules change. The resolver's buffers must hold the
// context's tags.
func (cr *contextResolver) trackOutput(context *Context, metricSampleContext metrics.MetricSampleContext, filterList filterlist.TagMatcher) {
	if !typeFiltersAtOutput(metricSampleContext.GetMetricType()) || (cr.sketchesPerTimestamp && becomesSketch(metricSampleContext)) {
		context.tagFilterFlags.set(hasUnfilterableType)
		cr.releaseFiltered(context)
		return
	}
	if context.tagFilterFlags&hasFilterableType != 0 {
		return
	}
	context.tagFilterFlags.set(hasFilterableType)
	if filterList != nil && context.filtersAtOutput() {
		cr.filterAtOutput(context, filterList)
	}
}

// filterAtOutput records the identity a context filters to, if a rule matches it. The resolver's buffers must hold the
// context's tags.
func (cr *contextResolver) filterAtOutput(context *Context, filterList filterlist.TagMatcher) {
	if tagMatcher, filter := filterList.ShouldStripTags(context.Name); filter {
		ref, removed := cr.acquireFiltered(context.Name, context.Host, tagMatcher)
		context.filteredIdentityRef = ref
		context.tagFilterFlags.set(routedToMerger)
		tlmFilteredTags.Add(float64(removed))
	}
}

// acquireFiltered filters the tags in the resolver's buffers, which must already be stored for the context, and
// returns a reference to the identity they filter to, and how many tags it removed.
func (cr *contextResolver) acquireFiltered(name, host string, tagMatcher func(tag string) bool) (uint32, int) {
	removed := cr.taggerBuffer.RetainFunc(tagMatcher) + cr.metricBuffer.RetainFunc(tagMatcher)
	key, taggerKey, metricKey := cr.keyGenerator.GenerateWithTags2(name, host, cr.taggerBuffer, cr.metricBuffer)
	cr.filteredIdentities.markFiltered(name)
	return cr.filteredIdentities.acquire(key, func() (*tags.Entry, *tags.Entry) {
		return cr.tagsCache.Insert(taggerKey, cr.taggerBuffer), cr.tagsCache.Insert(metricKey, cr.metricBuffer)
	}), removed
}

// releaseFiltered drops the context's reference to the identity it filters to, if any.
func (cr *contextResolver) releaseFiltered(context *Context) {
	if context.filteredIdentityRef != 0 {
		cr.filteredIdentities.release(context.filteredIdentityRef)
		context.filteredIdentityRef = 0
	}
}

// filteredIdentityOf returns the identity a context filters to at output. It is valid until the next context is
// tracked.
func (cr *contextResolver) filteredIdentityOf(context *Context) (*filteredIdentity, bool) {
	if context.filteredIdentityRef == 0 {
		return nil, false
	}
	return cr.filteredIdentities.get(context.filteredIdentityRef), true
}

// anyFilteredMetric returns whether any metric has been filtered at output since the Agent started.
func (cr *contextResolver) anyFilteredMetric() bool {
	return cr.filteredIdentities != nil && len(cr.filteredIdentities.filteredNames) > 0
}

// refilter updates the identity every context filters to at output, after the rules changed.
func (cr *contextResolver) refilter(filterList filterlist.TagMatcher) {
	if cr.filteredIdentities == nil || !cr.tagFilterEnabled {
		return
	}
	defer cr.taggerBuffer.Reset()
	defer cr.metricBuffer.Reset()
	type rule struct {
		tagMatcher func(tag string) bool
		filter     bool
	}
	rulesByName := make(map[string]rule)
	for _, entry := range cr.contextsByKey {
		context := entry.context
		var ref uint32
		if filterList != nil && context.filtersAtOutput() {
			r, ok := rulesByName[context.Name]
			if !ok {
				r.tagMatcher, r.filter = filterList.ShouldStripTags(context.Name)
				rulesByName[context.Name] = r
			}
			if r.filter {
				cr.taggerBuffer.Reset()
				cr.metricBuffer.Reset()
				cr.taggerBuffer.Append(context.taggerTags.Tags()...)
				cr.metricBuffer.Append(context.metricTags.Tags()...)
				ref, _ = cr.acquireFiltered(context.Name, context.Host, r.tagMatcher)
			}
		}
		cr.releaseFiltered(context)
		context.filteredIdentityRef = ref
	}
}

// becomesSketch returns whether a sample goes into a sketch: distributions and histogram buckets. Sketches stamped per
// second of submission (check_sampler_sketches_at_commit off) can't be merged across sources, so they aren't filtered.
func becomesSketch(metricSampleContext metrics.MetricSampleContext) bool {
	_, bucket := metricSampleContext.(*metrics.HistogramBucket)
	return bucket || metricSampleContext.GetMetricType() == metrics.DistributionType
}

// markFilteredMetrics routes the output of every context of the metrics filtered since the Agent started to the
// check output merger. It's called once every check sampler refiltered, so that the metrics any of them filtered
// are known.
func (cr *contextResolver) markFilteredMetrics() {
	if !cr.anyFilteredMetric() {
		return
	}
	for _, entry := range cr.contextsByKey {
		if cr.filteredIdentities.wasFiltered(entry.context.Name) {
			entry.context.tagFilterFlags.set(routedToMerger)
		}
	}
}
