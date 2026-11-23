// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/tags"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/metricname"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// Check samplers track every source of a metric with its full tags, so that state kept per source stays per source,
// and filter the tags that metric_tag_filterlist rules remove only from their output. Each context references the
// identity it filters to, which the contexts of every check sampler that filter to it share.

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

// filteredIdentityTable holds one filteredIdentity per filtered series, referenced by 1 + its index. The check
// samplers share one, as they are all used from the aggregator's run loop.
type filteredIdentityTable struct {
	identities []filteredIdentity
	free       []uint32
	byKey      map[ckey.ContextKey]uint32
}

func newFilteredIdentityTable() *filteredIdentityTable {
	return &filteredIdentityTable{byKey: make(map[ckey.ContextKey]uint32)}
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

// trackOutput applies tags filtering and saves the result in filteredIdentityTable.
// The resolver's buffers must hold the context's tags.
func (cr *contextResolver) trackOutput(context *Context, filterList metricname.TagMatcher) {
	if cr.filteredIdentities == nil || !cr.tagFilterEnabled || !typeFiltersAtOutput(context.mtype) {
		return
	}
	if tagList, filter := filterList.ShouldStripTags(context.Name); filter {
		ref, removed := cr.acquireFiltered(context.Name, context.Host, tagList)
		context.filteredIdentityRef = ref
		tlmFilteredTags.Add(float64(removed))
	}
}

// acquireFiltered filters the tags in the resolver's buffers, which must already be stored for the context, and
// returns a reference to the identity they filter to, and how many tags it removed.
func (cr *contextResolver) acquireFiltered(name, host string, tagList metricname.HashedMetricTagList) (uint32, int) {
	removed := cr.taggerBuffer.RetainFunc(tagList.KeepTag) + cr.metricBuffer.RetainFunc(tagList.KeepTag)
	key, taggerKey, metricKey := cr.keyGenerator.GenerateWithTags2(name, host, cr.taggerBuffer, cr.metricBuffer)
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

// filteredIdentityOf returns the identity a context filters to at output, or nil if it isn't filtered. It is valid
// until the next context is tracked.
func (cr *contextResolver) filteredIdentityOf(context *Context) *filteredIdentity {
	if context.filteredIdentityRef == 0 {
		return nil
	}
	return cr.filteredIdentities.get(context.filteredIdentityRef)
}

// refilter updates the identity every context filters to at output, after the rules changed.
func (cr *contextResolver) refilter(filterList metricname.TagMatcher) {
	if cr.filteredIdentities == nil || !cr.tagFilterEnabled {
		return
	}
	defer cr.taggerBuffer.Reset()
	defer cr.metricBuffer.Reset()
	type rule struct {
		tagList metricname.HashedMetricTagList
		filter  bool
	}
	rulesByName := make(map[string]rule)
	for _, entry := range cr.contextsByKey {
		context := entry.context
		var ref uint32
		if typeFiltersAtOutput(context.mtype) {
			r, ok := rulesByName[context.Name]
			if !ok {
				r.tagList, r.filter = filterList.ShouldStripTags(context.Name)
				rulesByName[context.Name] = r
			}
			if r.filter {
				cr.taggerBuffer.Reset()
				cr.metricBuffer.Reset()
				cr.taggerBuffer.Append(context.taggerTags.Tags()...)
				cr.metricBuffer.Append(context.metricTags.Tags()...)
				ref, _ = cr.acquireFiltered(context.Name, context.Host, r.tagList)
			}
		}
		cr.releaseFiltered(context)
		context.filteredIdentityRef = ref
	}
}
