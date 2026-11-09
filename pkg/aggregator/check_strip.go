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
// and strip the tags that metric_tag_filterlist rules remove only from their output. Each context references the
// identity it strips to, which the contexts of every check sampler that strip to it share.

// stripFlags record what check samplers do with a context's output.
type stripFlags uint8

const (
	// hasStrippableType is set once the context gets samples of a type whose tags can be stripped at output.
	hasStrippableType stripFlags = 1 << iota
	// hasUnstrippableType is set once the context gets samples of another type. Context keys don't include the
	// metric type, so samples of several types can share a context: then none of its output is stripped, from then
	// on, and a point the merger holds for it can be sent again for the same second.
	hasUnstrippableType
)

func (f *stripFlags) set(flag stripFlags) {
	*f |= flag
}

// stripsAtOutput returns whether check samplers strip the context's output when a rule matches it.
func (c *Context) stripsAtOutput() bool {
	return c.stripFlags&(hasStrippableType|hasUnstrippableType) == hasStrippableType
}

// mergesAtOutput returns whether the context's output goes to the check output merger: a rule strips it.
func (c *Context) mergesAtOutput() bool {
	return c.strippedIdentityRef != 0
}

// typeStripsAtOutput returns whether check samplers can strip the tags of a metric type from their output: the types
// whose sources' points add up.
func typeStripsAtOutput(mtype metrics.MetricType) bool {
	switch mtype {
	case metrics.DistributionType, metrics.CounterType:
		return true
	}
	return false
}

// strippedIdentity is the identity of a context after metric_tag_filterlist rules: its key and the tags left
// after stripping, with the same name and host.
type strippedIdentity struct {
	key        ckey.ContextKey
	taggerTags *tags.Entry
	metricTags *tags.Entry
	refs       uint32
}

// Tags returns the tags left after stripping.
func (s *strippedIdentity) Tags() tagset.CompositeTags {
	return tagset.NewCompositeTags(s.taggerTags.Tags(), s.metricTags.Tags())
}

// strippedIdentityTable holds one strippedIdentity per stripped series, referenced by 1 + its index. The check
// samplers share one, as they are all used from the aggregator's run loop.
type strippedIdentityTable struct {
	identities []strippedIdentity
	free       []uint32
	byKey      map[ckey.ContextKey]uint32
}

func newStrippedIdentityTable() *strippedIdentityTable {
	return &strippedIdentityTable{byKey: make(map[ckey.ContextKey]uint32)}
}

// acquire returns a reference to the identity with the key, creating it with newTags if there is none.
func (t *strippedIdentityTable) acquire(key ckey.ContextKey, newTags func() (taggerTags, metricTags *tags.Entry)) uint32 {
	if ref, ok := t.byKey[key]; ok {
		t.identities[ref-1].refs++
		return ref
	}
	identity := strippedIdentity{key: key, refs: 1}
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
func (t *strippedIdentityTable) release(ref uint32) {
	identity := &t.identities[ref-1]
	if identity.refs--; identity.refs > 0 {
		return
	}
	identity.taggerTags.Release()
	identity.metricTags.Release()
	delete(t.byKey, identity.key)
	*identity = strippedIdentity{}
	t.free = append(t.free, ref)
}

// get returns the identity for a reference. It is valid until the next acquire.
func (t *strippedIdentityTable) get(ref uint32) *strippedIdentity {
	return &t.identities[ref-1]
}

// trackOutput records what a sample adds to its context's output. The first sample of a type that can be stripped
// strips the context if a rule matches: the identity only depends on the context, so the rule is looked up once
// (ShouldStripTags allocates), and restrip updates contexts when rules change. The resolver's buffers must hold the
// context's tags.
func (cr *contextResolver) trackOutput(context *Context, metricSampleContext metrics.MetricSampleContext, filterList filterlist.TagMatcher) {
	if !typeStripsAtOutput(metricSampleContext.GetMetricType()) || (cr.sketchesPerTimestamp && becomesSketch(metricSampleContext)) {
		context.stripFlags.set(hasUnstrippableType)
		cr.releaseStripped(context)
		return
	}
	if context.stripFlags&hasStrippableType != 0 {
		return
	}
	context.stripFlags.set(hasStrippableType)
	if filterList != nil && context.stripsAtOutput() {
		cr.strip(context, filterList)
	}
}

// strip records the identity a context strips to, if a rule matches it. The resolver's buffers must hold the
// context's tags.
func (cr *contextResolver) strip(context *Context, filterList filterlist.TagMatcher) {
	if tagMatcher, filter := filterList.ShouldStripTags(context.Name); filter {
		ref, removed := cr.acquireStripped(context.Name, context.Host, tagMatcher)
		context.strippedIdentityRef = ref
		tlmFilteredTags.Add(float64(removed))
	}
}

// acquireStripped strips the tags in the resolver's buffers, which must already be stored for the context, and
// returns a reference to the identity they strip to, and how many tags it removed.
func (cr *contextResolver) acquireStripped(name, host string, tagMatcher func(tag string) bool) (uint32, int) {
	removed := cr.taggerBuffer.RetainFunc(tagMatcher) + cr.metricBuffer.RetainFunc(tagMatcher)
	key, taggerKey, metricKey := cr.keyGenerator.GenerateWithTags2(name, host, cr.taggerBuffer, cr.metricBuffer)
	return cr.stripped.acquire(key, func() (*tags.Entry, *tags.Entry) {
		return cr.tagsCache.Insert(taggerKey, cr.taggerBuffer), cr.tagsCache.Insert(metricKey, cr.metricBuffer)
	}), removed
}

// releaseStripped drops the context's reference to the identity it strips to, if any.
func (cr *contextResolver) releaseStripped(context *Context) {
	if context.strippedIdentityRef != 0 {
		cr.stripped.release(context.strippedIdentityRef)
		context.strippedIdentityRef = 0
	}
}

// strippedOf returns the identity a context strips to at output. It is valid until the next context is tracked.
func (cr *contextResolver) strippedOf(context *Context) (*strippedIdentity, bool) {
	if context.strippedIdentityRef == 0 {
		return nil, false
	}
	return cr.stripped.get(context.strippedIdentityRef), true
}

// restrip updates the identity every context strips to at output, after the rules changed.
func (cr *contextResolver) restrip(filterList filterlist.TagMatcher) {
	if cr.stripped == nil || !cr.tagFilterEnabled {
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
		if filterList != nil && context.stripsAtOutput() {
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
				ref, _ = cr.acquireStripped(context.Name, context.Host, r.tagMatcher)
			}
		}
		cr.releaseStripped(context)
		context.strippedIdentityRef = ref
	}
}

// becomesSketch returns whether a sample goes into a sketch: distributions and histogram buckets. Sketches stamped per
// second of submission (check_sampler_sketches_at_commit off) can't be merged across sources, so they aren't stripped.
func becomesSketch(metricSampleContext metrics.MetricSampleContext) bool {
	_, bucket := metricSampleContext.(*metrics.HistogramBucket)
	return bucket || metricSampleContext.GetMetricType() == metrics.DistributionType
}
