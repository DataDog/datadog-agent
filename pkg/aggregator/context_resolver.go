// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"io"
	"unsafe"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	filterlist "github.com/DataDog/datadog-agent/comp/filterlist/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/tags"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/size"
)

// Context holds the elements that form a context, and can be serialized into a context key
type Context struct {
	Name       string
	Host       string
	mtype      metrics.MetricType
	taggerTags *tags.Entry
	metricTags *tags.Entry
	noIndex    bool
	stripFlags contextOutput
	source     metrics.MetricSource

	strippedIdentityRef uint32
}

// stripsAtOutput returns whether check samplers can strip the context's output: once it gets samples of a type that
// can be stripped, whichever type created it.
func (c *Context) stripsAtOutput() bool {
	return c.stripFlags&strippable != 0
}

// mergesAtOutput returns whether the context's output that can be stripped goes to the check output merger, stamped
// at commit: its metric has been stripped since the Agent started.
func (c *Context) mergesAtOutput() bool {
	return c.stripFlags&strippedMetric != 0 && c.stripsAtOutput()
}

// mergesSeries returns whether the context's series go to the check output merger: none of its samples are of a type
// whose series can't be merged.
func (c *Context) mergesSeries() bool {
	return c.mergesAtOutput() && c.stripFlags&unmergeableSeries == 0
}

// contextOutput are flags on what check samplers do with a context's output.
type contextOutput uint8

const (
	// strippedMetric is set for every context of a metric that has been stripped at output since the Agent
	// started. Its output that can be stripped is stamped at commit and goes to the check output merger, stripped
	// of tags while a rule applies and with its full tags otherwise: a sketch stamped at commit can't land in a
	// second the merger has already sent, since a commit after a flush is stamped at or after its cutoff, and the
	// merger merges two commits in one second.
	strippedMetric contextOutput = 1 << iota
	// strippable is set once a context gets samples of a type that can be stripped at output. Context keys don't
	// include the metric type, so samples of several types can share a context, whichever came first.
	strippable
	// unmergeableSeries is set once a context gets samples of a type whose series can't be merged, such as an
	// ordinary histogram's statistics or an explicit-timestamp count: its series are never merged.
	unmergeableSeries
)

func (o *contextOutput) set(flag contextOutput, on bool) {
	if on {
		*o |= flag
	} else {
		*o &^= flag
	}
}

// strippedIdentity is the identity of a context after metric_tag_filterlist rules: its key and the tags left
// after stripping, with the same name and host. Every source context that strips to it shares it, across check
// samplers.
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

// strippedIdentityTable holds one strippedIdentity per stripped series, referenced by 1 + its index, and the names
// of the metrics that have been stripped. The check samplers share one, as they are all used from the aggregator's
// run loop.
type strippedIdentityTable struct {
	identities []strippedIdentity
	free       []uint32
	byKey      map[ckey.ContextKey]uint32
	// strippedNames are the metrics stripped since the Agent started: their output that can be stripped stays
	// stamped at commit, and goes to the check output merger.
	strippedNames map[string]struct{}
}

func newStrippedIdentityTable() *strippedIdentityTable {
	return &strippedIdentityTable{byKey: make(map[ckey.ContextKey]uint32), strippedNames: make(map[string]struct{})}
}

// wasStripped returns whether a metric has been stripped since the Agent started.
func (t *strippedIdentityTable) wasStripped(name string) bool {
	if t == nil {
		return false
	}
	_, ok := t.strippedNames[name]
	return ok
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

// resolvedMetricView exposes the final metric identity to the anomaly-detection
// observer. The view itself is valid only for the synchronous duration of
// ObserveMetric, but its CompositeTags are immutable and may be retained by the
// observer after the call returns.
type resolvedMetricView struct {
	sample *metrics.MetricSample
	host   string
	tags   tagset.CompositeTags
}

func (v resolvedMetricView) GetName() string                   { return v.sample.GetName() }
func (v resolvedMetricView) GetValue() float64                 { return v.sample.GetValue() }
func (v resolvedMetricView) GetTags() tagset.CompositeTags     { return v.tags }
func (v resolvedMetricView) GetHost() string                   { return v.host }
func (v resolvedMetricView) GetTimestampUnix() int64           { return v.sample.GetTimestampUnix() }
func (v resolvedMetricView) GetSampleRate() float64            { return v.sample.GetSampleRate() }
func (v resolvedMetricView) GetMetricType() metrics.MetricType { return v.sample.GetMetricType() }

type resolverEntry struct {
	lastSeen int64
	context  *Context
}

const (
	// ContextSizeInBytes is the size of a context in bytes
	// We count the size of the context key with the context.
	ContextSizeInBytes = int(unsafe.Sizeof(Context{})) + int(unsafe.Sizeof(ckey.ContextKey(0)))
)

// Tags returns tags for the context.
func (c *Context) Tags() tagset.CompositeTags {
	return tagset.NewCompositeTags(c.taggerTags.Tags(), c.metricTags.Tags())
}

func (c *Context) release() {
	c.taggerTags.Release()
	c.metricTags.Release()
}

// SizeInBytes returns the size of the context in bytes
func (c *Context) SizeInBytes() int {
	return ContextSizeInBytes
}

// DataSizeInBytes returns the size of the context data in bytes
func (c *Context) DataSizeInBytes() int {
	return len(c.Name) + len(c.Host) + c.taggerTags.DataSizeInBytes() + c.metricTags.DataSizeInBytes()
}

// Make sure we implement the interface
var _ size.HasSizeInBytes = &Context{}

// contextResolver allows tracking and expiring contexts
type contextResolver struct {
	id               string
	contextsByKey    map[ckey.ContextKey]resolverEntry
	seendByMtype     []bool
	countsByMtype    []uint64
	bytesByMtype     []uint64
	dataBytesByMtype []uint64
	tagsCache        *tags.Store
	tagger           tagger.Component
	keyGenerator     *ckey.KeyGenerator
	taggerBuffer     *tagset.HashingTagsAccumulator
	metricBuffer     *tagset.HashingTagsAccumulator
	// tagFilterCache maps a pre-filter contextKey to the post-filter (contextKey, taggerKey, metricKey).
	// This avoids repeated RetainFunc calls for metrics we have already processed.
	tagFilterCache *tagFilterCache
	// tagFilterEnabled controls whether metric_tag_filterlist tag stripping is applied.
	// True when data_plane.enabled is true, or metric_tag_filterlist_adp_only is false.
	tagFilterEnabled bool
	// stripAtOutput keeps the full tags of contexts, recording the identity they strip to in stripped, instead of
	// tracking them under the stripped context: check samplers strip their output, so that state kept per source
	// stays per source.
	stripAtOutput bool
	stripped      *strippedIdentityTable
}

// generateContextKey generates the contextKey associated with the context of the metricSample
func (cr *contextResolver) generateContextKey(metricSampleContext metrics.MetricSampleContext) (ckey.ContextKey, ckey.TagsKey, ckey.TagsKey) {
	return cr.keyGenerator.GenerateWithTags2(metricSampleContext.GetName(), metricSampleContext.GetHost(), cr.taggerBuffer, cr.metricBuffer)
}

// tagFilterEnabled returns whether metric_tag_filterlist rules apply: when data_plane.enabled is true, or
// metric_tag_filterlist_adp_only is false.
func tagFilterEnabled() bool {
	cfg := pkgconfigsetup.Datadog()
	return cfg.GetBool("data_plane.enabled") || !cfg.GetBool("metric_tag_filterlist_adp_only")
}

func newContextResolver(tagger tagger.Component, cache *tags.Store, id string) *contextResolver {
	cfg := pkgconfigsetup.Datadog()
	return &contextResolver{
		id:               id,
		contextsByKey:    make(map[ckey.ContextKey]resolverEntry),
		seendByMtype:     make([]bool, metrics.NumMetricTypes),
		countsByMtype:    make([]uint64, metrics.NumMetricTypes),
		bytesByMtype:     make([]uint64, metrics.NumMetricTypes),
		dataBytesByMtype: make([]uint64, metrics.NumMetricTypes),
		tagsCache:        cache,
		tagger:           tagger,
		keyGenerator:     ckey.NewKeyGenerator(),
		taggerBuffer:     tagset.NewHashingTagsAccumulator(),
		metricBuffer:     tagset.NewHashingTagsAccumulator(),
		tagFilterCache:   newTagFilterCache(cfg.GetInt("aggregator_tag_filter_cache_capacity")),
		tagFilterEnabled: tagFilterEnabled(),
	}
}

// trackContext returns the contextKey associated with the context of the metricSample and tracks that context
func (cr *contextResolver) trackContext(metricSampleContext metrics.MetricSampleContext, timestamp int64, filterList filterlist.TagMatcher) ckey.ContextKey {
	metricSampleContext.GetTags(cr.taggerBuffer, cr.metricBuffer, cr.tagger) // tags here are not sorted and can contain duplicates

	defer cr.taggerBuffer.Reset()
	defer cr.metricBuffer.Reset()

	contextKey, taggerKey, metricKey := cr.generateContextKey(metricSampleContext) // the generator will remove duplicates (and doesn't mind the order)

	if !cr.stripAtOutput && filterList != nil && cr.tagFilterEnabled && shouldAggregateTags(metricSampleContext) {
		if tagMatcher, filter := filterList.ShouldStripTags(metricSampleContext.GetName()); filter {
			contextKey, taggerKey, metricKey = cr.filterTags(metricSampleContext, tagMatcher, contextKey)
		}
	}
	tracksOutput := cr.stripAtOutput && cr.tagFilterEnabled

	if entry, ok := cr.contextsByKey[contextKey]; !ok {
		mtype := metricSampleContext.GetMetricType()
		context := &Context{
			Name:       metricSampleContext.GetName(),
			taggerTags: cr.tagsCache.Insert(taggerKey, cr.taggerBuffer),
			metricTags: cr.tagsCache.Insert(metricKey, cr.metricBuffer),
			Host:       metricSampleContext.GetHost(),
			mtype:      mtype,
			noIndex:    metricSampleContext.IsNoIndex(),
			source:     metricSampleContext.GetSource(),
		}
		if tracksOutput {
			context.stripFlags.set(strippedMetric, cr.stripped.wasStripped(context.Name))
			cr.trackOutput(context, metricSampleContext, filterList)
		}
		cr.contextsByKey[contextKey] = resolverEntry{
			lastSeen: timestamp,
			context:  context,
		}

		cr.seendByMtype[mtype] = true
		cr.countsByMtype[mtype]++
		cr.bytesByMtype[mtype] += uint64(context.SizeInBytes())
		cr.dataBytesByMtype[mtype] += uint64(context.DataSizeInBytes())
	} else {
		if tracksOutput {
			cr.trackOutput(entry.context, metricSampleContext, filterList)
		}
		// We can't assign to a field of a struct contained in map
		cr.contextsByKey[contextKey] = resolverEntry{
			lastSeen: timestamp,
			context:  entry.context,
		}
	}

	return contextKey
}

// trackOutput records what a sample adds to its context's output. The first sample of a type that can be stripped
// makes the context strippable, and strips it if a rule matches: the identity only depends on the context, so the
// rule is looked up once (ShouldStripTags allocates), and restrip updates contexts when rules change. A sample of a
// type whose series can't be merged keeps the context's series from being merged. The resolver's buffers must hold
// the context's tags.
func (cr *contextResolver) trackOutput(context *Context, metricSampleContext metrics.MetricSampleContext, filterList filterlist.TagMatcher) {
	if !stripsAtOutput(metricSampleContext.GetMetricType()) {
		context.stripFlags.set(unmergeableSeries, true)
		return
	}
	if context.stripFlags&strippable != 0 {
		return
	}
	context.stripFlags.set(strippable, true)
	if filterList != nil && context.strippedIdentityRef == 0 {
		cr.strip(context, filterList)
		context.stripFlags.set(strippedMetric, cr.stripped.wasStripped(context.Name))
	}
}

// strip records the identity a context strips to, if a rule matches it. The resolver's buffers must hold the
// context's tags.
func (cr *contextResolver) strip(context *Context, filterList filterlist.TagMatcher) {
	if tagMatcher, filter := filterList.ShouldStripTags(context.Name); filter {
		stripped, removed := cr.acquireStripped(context.Name, context.Host, tagMatcher)
		context.strippedIdentityRef = stripped
		tlmFilteredTags.Add(float64(removed))
	}
}

// acquireStripped strips the tags in the resolver's buffers, which must already be stored for the context, and
// returns a reference to the identity they strip to, and how many tags it removed.
func (cr *contextResolver) acquireStripped(name, host string, tagMatcher func(tag string) bool) (uint32, int) {
	removed := cr.taggerBuffer.RetainFunc(tagMatcher) + cr.metricBuffer.RetainFunc(tagMatcher)
	key, taggerKey, metricKey := cr.keyGenerator.GenerateWithTags2(name, host, cr.taggerBuffer, cr.metricBuffer)
	if cr.stripped == nil {
		cr.stripped = newStrippedIdentityTable()
	}
	cr.stripped.strippedNames[name] = struct{}{}
	return cr.stripped.acquire(key, func() (*tags.Entry, *tags.Entry) {
		return cr.tagsCache.Insert(taggerKey, cr.taggerBuffer), cr.tagsCache.Insert(metricKey, cr.metricBuffer)
	}), removed
}

// strippedOf returns the identity a context strips to at output. It is valid until the next context is tracked.
func (cr *contextResolver) strippedOf(context *Context) (*strippedIdentity, bool) {
	if context.strippedIdentityRef == 0 {
		return nil, false
	}
	return cr.stripped.get(context.strippedIdentityRef), true
}

// anyStrippedMetric returns whether any metric has been stripped at output since the Agent started.
func (cr *contextResolver) anyStrippedMetric() bool {
	return cr.stripped != nil && len(cr.stripped.strippedNames) > 0
}

// restrip updates the identity every context strips to at output, after the rules changed.
func (cr *contextResolver) restrip(filterList filterlist.TagMatcher) {
	if !cr.stripAtOutput || !cr.tagFilterEnabled {
		return
	}
	defer cr.taggerBuffer.Reset()
	defer cr.metricBuffer.Reset()
	// ShouldStripTags allocates: look each metric's rule up once.
	type rule struct {
		tagMatcher func(tag string) bool
		filter     bool
	}
	rules := make(map[string]rule)
	for _, entry := range cr.contextsByKey {
		context := entry.context
		var stripped uint32
		if filterList != nil && cr.tagFilterEnabled && context.stripsAtOutput() {
			r, ok := rules[context.Name]
			if !ok {
				r.tagMatcher, r.filter = filterList.ShouldStripTags(context.Name)
				rules[context.Name] = r
			}
			if tagMatcher, filter := r.tagMatcher, r.filter; filter {
				cr.taggerBuffer.Reset()
				cr.metricBuffer.Reset()
				cr.taggerBuffer.Append(context.taggerTags.Tags()...)
				cr.metricBuffer.Append(context.metricTags.Tags()...)
				stripped, _ = cr.acquireStripped(context.Name, context.Host, tagMatcher)
			}
		}
		if context.strippedIdentityRef != 0 {
			cr.stripped.release(context.strippedIdentityRef)
		}
		context.strippedIdentityRef = stripped
	}
}

// markStrippedMetrics marks the contexts of metrics stripped since the Agent started. It's called after every check sampler
// restripped, so that the metrics any of them stripped are known.
func (cr *contextResolver) markStrippedMetrics() {
	if !cr.anyStrippedMetric() {
		return
	}
	for _, entry := range cr.contextsByKey {
		entry.context.stripFlags.set(strippedMetric, cr.stripped.wasStripped(entry.context.Name))
	}
}

// shouldAggregateTags returns true if the tag for the given metric should be considered
// for aggregation in a time sampler: Distribution, and Counter (dogstatsd counts).
func shouldAggregateTags(metricSampleContext metrics.MetricSampleContext) bool {
	mtype := metricSampleContext.GetMetricType()
	return mtype == metrics.DistributionType ||
		mtype == metrics.CounterType
}

// stripsAtOutput returns whether check samplers strip the tags of a metric type from their output: types whose
// sources' points add up, so that the check output merger can merge them. Monotonic counts keep state per source,
// which stays per source because check samplers aggregate sources with their full tags.
func stripsAtOutput(mtype metrics.MetricType) bool {
	switch mtype {
	case metrics.DistributionType, metrics.CounterType, metrics.CountType, metrics.MonotonicCountType:
		return true
	}
	return false
}

// filterTags filters tags from the context that match the given tagMatcher.
// Results are cached in tagFilterCache so repeated calls for the same pre-filter
// context key skip the RetainFunc work.
func (cr *contextResolver) filterTags(
	metricSampleContext metrics.MetricSampleContext,
	tagMatcher func(tag string) bool,
	contextKey ckey.ContextKey,
) (ckey.ContextKey, ckey.TagsKey, ckey.TagsKey) {

	if cached, ok := cr.tagFilterCache.get(contextKey); ok {
		// Cache hit: reuse previously computed post-filter keys, skip RetainFunc.
		tlmFilteredTags.Add(float64(cached.removedTags))
		tlmFilteredTagsCacheHit.Inc()

		return cached.contextKey, cached.taggerKey, cached.metricKey
	}

	// Cache miss: filter tags and compute post-filter keys.
	// Currently only distributions are supported, filter out tags if it is configured to remove tags for this given
	// metric.
	removedTagger := cr.taggerBuffer.RetainFunc(tagMatcher)
	removedMetric := cr.metricBuffer.RetainFunc(tagMatcher)
	removed := removedTagger + removedMetric
	tlmFilteredTags.Add(float64(removed))
	filteredContextKey, filteredTaggerKey, filteredMetricKey := cr.generateContextKey(metricSampleContext) // the generator will remove duplicates (and doesn't mind the order)
	cr.tagFilterCache.add(contextKey, tagFilterCacheEntry{
		contextKey:  filteredContextKey,
		taggerKey:   filteredTaggerKey,
		metricKey:   filteredMetricKey,
		removedTags: removed,
	})
	tlmFilteredTagsCacheMiss.Inc()

	return filteredContextKey, filteredTaggerKey, filteredMetricKey
}

func (cr *contextResolver) get(key ckey.ContextKey) (*Context, bool) {
	ctx, found := cr.contextsByKey[key]
	return ctx.context, found
}

func (cr *contextResolver) length() int {
	return len(cr.contextsByKey)
}

func (cr *contextResolver) remove(expiredContextKey ckey.ContextKey) {
	context := cr.contextsByKey[expiredContextKey].context
	delete(cr.contextsByKey, expiredContextKey)

	cr.tagFilterCache.delete(expiredContextKey)

	if context != nil {
		cr.countsByMtype[context.mtype]--
		cr.bytesByMtype[context.mtype] -= uint64(context.SizeInBytes())
		cr.dataBytesByMtype[context.mtype] -= uint64(context.DataSizeInBytes())
		if context.strippedIdentityRef != 0 {
			cr.stripped.release(context.strippedIdentityRef)
		}
		context.release()
	}
}

func (cr *contextResolver) updateMetrics(countsByMTypeGauge telemetry.Gauge, bytesByMTypeGauge telemetry.Gauge) {
	for i := 0; i < int(metrics.NumMetricTypes); i++ {
		count := cr.countsByMtype[i]
		bytes := cr.bytesByMtype[i]
		dataBytes := cr.dataBytesByMtype[i]
		mtype := metrics.MetricType(i).String()

		// Limit un-needed cardinality (especially because each check has its own resolver)
		if !cr.seendByMtype[i] {
			continue
		}
		countsByMTypeGauge.WithValues(cr.id, mtype).Set(float64(count))
		bytesByMTypeGauge.Set(float64(bytes), cr.id, mtype, tags.BytesKindStruct)
		bytesByMTypeGauge.Set(float64(dataBytes), cr.id, mtype, tags.BytesKindData)
	}
}

func (cr *contextResolver) release() {
	for _, c := range cr.contextsByKey {
		if c.context.strippedIdentityRef != 0 {
			cr.stripped.release(c.context.strippedIdentityRef)
		}
		c.context.release()
	}
	cr.clearTagFilterCache()
}

func (cr *contextResolver) clearTagFilterCache() {
	cr.tagFilterCache.clear()
}

func (cr *contextResolver) sendOriginTelemetry(timestamp float64, series metrics.SerieSink, hostname string, constTags []string) {
	// Within the contextResolver, each set of tags is represented by a unique pointer.
	perOrigin := map[*tags.Entry]uint64{}
	for _, cx := range cr.contextsByKey {
		perOrigin[cx.context.taggerTags]++
	}

	// We send metrics directly to the sink, instead of using
	// pkg/telemetry for a few reasons:
	//
	// 1. We can send full set of tagger tags for higher level
	//    aggregations (pod, namespace, etc). pkg/telemetry only
	//    allows a fixed set of tags.
	// 2. Avoid the need to manually create and delete tag values
	//    inside a telemetry Gauge.
	// 3. Cardinality is automatically limited to origins verified by
	//    the tagger (although broken applications sending invalid
	//    origin id would coalesce to no origin, making this less
	//    useful for troubleshooting).
	for entry, count := range perOrigin {
		series.Append(&metrics.Serie{
			Name:   "datadog.agent.aggregator.dogstatsd_contexts_by_origin",
			Host:   hostname,
			Tags:   tagset.NewCompositeTags(constTags, entry.Tags()),
			MType:  metrics.APIGaugeType,
			Points: []metrics.Point{{Ts: timestamp, Value: float64(count)}},
		})
	}
}

// timestampContextResolver allows tracking and expiring contexts based on time.
type timestampContextResolver struct {
	resolver *contextResolver

	contextExpireTime int64
	counterExpireTime int64
}

func newTimestampContextResolver(tagger tagger.Component, cache *tags.Store, id string, contextExpireTime, counterExpireTime int64) *timestampContextResolver {
	return &timestampContextResolver{
		resolver: newContextResolver(tagger, cache, id),

		contextExpireTime: contextExpireTime,
		counterExpireTime: counterExpireTime,
	}
}

// trackContext returns the contextKey associated with the context of the metricSample and tracks that context
func (cr *timestampContextResolver) trackContext(metricSampleContext metrics.MetricSampleContext, currentTimestamp int64, filterList filterlist.TagMatcher) ckey.ContextKey {
	contextKey := cr.resolver.trackContext(metricSampleContext, currentTimestamp, filterList)
	return contextKey
}

func (cr *timestampContextResolver) length() int {
	return cr.resolver.length()
}

func (cr *timestampContextResolver) countsByMtype() []uint64 {
	return cr.resolver.countsByMtype
}

func (cr *timestampContextResolver) get(key ckey.ContextKey) (*Context, bool) {
	return cr.resolver.get(key)
}

// expireContexts cleans up the contexts that haven't been tracked since the given timestamp
func (cr *timestampContextResolver) expireContexts(timestamp int64) {
	for ck, entry := range cr.resolver.contextsByKey {
		ttl := cr.contextExpireTime
		if entry.context.mtype == metrics.CounterType {
			ttl = cr.counterExpireTime
		}
		if entry.lastSeen+ttl < timestamp {
			cr.resolver.remove(ck)
		}
	}
}

func (cr *timestampContextResolver) sendOriginTelemetry(timestamp float64, series metrics.SerieSink, hostname string, tags []string) {
	cr.resolver.sendOriginTelemetry(timestamp, series, hostname, tags)
}

func (cr *timestampContextResolver) dumpContexts(dest io.Writer) error {
	return cr.resolver.dumpContexts(dest)
}

func (cr *timestampContextResolver) updateMetrics(countsByMTypeGauge telemetry.Gauge, bytesByMTypeGauge telemetry.Gauge) {
	cr.resolver.updateMetrics(countsByMTypeGauge, bytesByMTypeGauge)
}

// countBasedContextResolver allows tracking and expiring contexts based on the number
// of calls of `expireContexts`.
type countBasedContextResolver struct {
	resolver            *contextResolver
	expireCount         int64
	expireCountInterval int64
}

func newCountBasedContextResolver(expireCountInterval int, cache *tags.Store, tagger tagger.Component, id string) *countBasedContextResolver {
	return &countBasedContextResolver{
		resolver:            newContextResolver(tagger, cache, id),
		expireCount:         0,
		expireCountInterval: int64(expireCountInterval),
	}
}

// length returns the number of contexts tracked by the resolver
func (cr *countBasedContextResolver) length() int {
	return cr.resolver.length()
}

func (cr *countBasedContextResolver) updateMetrics(countsByMTypeGauge telemetry.Gauge, bytesByMTypeGauge telemetry.Gauge) {
	cr.resolver.updateMetrics(countsByMTypeGauge, bytesByMTypeGauge)
}

// trackContext returns the contextKey associated with the context of the metricSample and tracks that context
func (cr *countBasedContextResolver) trackContext(metricSampleContext metrics.MetricSampleContext, filterList filterlist.TagMatcher) ckey.ContextKey {
	contextKey := cr.resolver.trackContext(metricSampleContext, cr.expireCount, filterList)
	return contextKey
}

func (cr *countBasedContextResolver) get(key ckey.ContextKey) (*Context, bool) {
	return cr.resolver.get(key)
}

// expireContexts cleans up the contexts that haven't been tracked since `expirationCount`
// call to `expireContexts` and returns the associated contextKeys
func (cr *countBasedContextResolver) expireContexts() []ckey.ContextKey {
	var keys []ckey.ContextKey
	for key, entry := range cr.resolver.contextsByKey {
		index := entry.lastSeen
		if index <= cr.expireCount-cr.expireCountInterval {
			keys = append(keys, key)
			cr.resolver.remove(key)
		}
	}
	cr.expireCount++
	return keys
}

func (cr *countBasedContextResolver) release() {
	cr.resolver.release()
}
