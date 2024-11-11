// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"io"
	"unsafe"

	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/tags"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util"
)

// Context holds the elements that form a context, and can be serialized into a context key
type Context struct {
	Name       string
	Host       string
	mtype      metrics.MetricType
	taggerTags *tags.Entry
	metricTags *tags.Entry
	noIndex    bool
	source     metrics.MetricSource
}

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
var _ util.HasSizeInBytes = &Context{}

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
}

// generateContextKey generates the contextKey associated with the context of the metricSample
func (cr *contextResolver) generateContextKey(metricSampleContext metrics.MetricSampleContext) (ckey.ContextKey, ckey.TagsKey, ckey.TagsKey) {
	return cr.keyGenerator.GenerateWithTags2(metricSampleContext.GetName(), metricSampleContext.GetHost(), cr.taggerBuffer, cr.metricBuffer)
}

func newContextResolver(tagger tagger.Component, cache *tags.Store, id string) *contextResolver {
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
	}
}

// trackContext returns the contextKey associated with the context of the metricSample and tracks that context
func (cr *contextResolver) trackContext(metricSampleContext metrics.MetricSampleContext, timestamp int64) ckey.ContextKey {
	metricSampleContext.GetTags(cr.taggerBuffer, cr.metricBuffer, cr.tagger.EnrichTags) // tags here are not sorted and can contain duplicates
	defer cr.taggerBuffer.Reset()
	defer cr.metricBuffer.Reset()

	contextKey, taggerKey, metricKey := cr.generateContextKey(metricSampleContext) // the generator will remove duplicates (and doesn't mind the order)

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
		cr.contextsByKey[contextKey] = resolverEntry{
			lastSeen: timestamp,
			context:  context,
		}

		cr.seendByMtype[mtype] = true
		cr.countsByMtype[mtype]++
		cr.bytesByMtype[mtype] += uint64(context.SizeInBytes())
		cr.dataBytesByMtype[mtype] += uint64(context.DataSizeInBytes())
	} else {
		// We can't assign to a field of a struct contained in map
		cr.contextsByKey[contextKey] = resolverEntry{
			lastSeen: timestamp,
			context:  entry.context,
		}
	}

	return contextKey
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

	if context != nil {
		cr.countsByMtype[context.mtype]--
		cr.bytesByMtype[context.mtype] -= uint64(context.SizeInBytes())
		cr.dataBytesByMtype[context.mtype] -= uint64(context.DataSizeInBytes())
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
		bytesByMTypeGauge.Set(float64(bytes), cr.id, mtype, util.BytesKindStruct)
		bytesByMTypeGauge.Set(float64(dataBytes), cr.id, mtype, util.BytesKindData)
	}
}

func (cr *contextResolver) release() {
	for _, c := range cr.contextsByKey {
		c.context.release()
	}
}

//nolint:revive // TODO(AML) Fix revive linter
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
func (cr *timestampContextResolver) trackContext(metricSampleContext metrics.MetricSampleContext, currentTimestamp int64) ckey.ContextKey {
	contextKey := cr.resolver.trackContext(metricSampleContext, currentTimestamp)
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
func (cr *countBasedContextResolver) trackContext(metricSampleContext metrics.MetricSampleContext) ckey.ContextKey {
	contextKey := cr.resolver.trackContext(metricSampleContext, cr.expireCount)
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
