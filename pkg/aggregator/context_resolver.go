// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"io"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/tags"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagger"
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
	panic("not called")
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
	contextsByKey    map[ckey.ContextKey]*Context
	seendByMtype     []bool
	countsByMtype    []uint64
	bytesByMtype     []uint64
	dataBytesByMtype []uint64
	tagsCache        *tags.Store
	keyGenerator     *ckey.KeyGenerator
	taggerBuffer     *tagset.HashingTagsAccumulator
	metricBuffer     *tagset.HashingTagsAccumulator
}

// generateContextKey generates the contextKey associated with the context of the metricSample
func (cr *contextResolver) generateContextKey(metricSampleContext metrics.MetricSampleContext) (ckey.ContextKey, ckey.TagsKey, ckey.TagsKey) {
	return cr.keyGenerator.GenerateWithTags2(metricSampleContext.GetName(), metricSampleContext.GetHost(), cr.taggerBuffer, cr.metricBuffer)
}

func newContextResolver(cache *tags.Store, id string) *contextResolver {
	return &contextResolver{
		id:               id,
		contextsByKey:    make(map[ckey.ContextKey]*Context),
		seendByMtype:     make([]bool, metrics.NumMetricTypes),
		countsByMtype:    make([]uint64, metrics.NumMetricTypes),
		bytesByMtype:     make([]uint64, metrics.NumMetricTypes),
		dataBytesByMtype: make([]uint64, metrics.NumMetricTypes),
		tagsCache:        cache,
		keyGenerator:     ckey.NewKeyGenerator(),
		taggerBuffer:     tagset.NewHashingTagsAccumulator(),
		metricBuffer:     tagset.NewHashingTagsAccumulator(),
	}
}

// trackContext returns the contextKey associated with the context of the metricSample and tracks that context
func (cr *contextResolver) trackContext(metricSampleContext metrics.MetricSampleContext) ckey.ContextKey {
	metricSampleContext.GetTags(cr.taggerBuffer, cr.metricBuffer, tagger.EnrichTags) // tags here are not sorted and can contain duplicates
	defer cr.taggerBuffer.Reset()
	defer cr.metricBuffer.Reset()

	contextKey, taggerKey, metricKey := cr.generateContextKey(metricSampleContext) // the generator will remove duplicates (and doesn't mind the order)

	if _, ok := cr.contextsByKey[contextKey]; !ok {
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
		cr.contextsByKey[contextKey] = context
		cr.seendByMtype[mtype] = true
		cr.countsByMtype[mtype]++
		cr.bytesByMtype[mtype] += uint64(context.SizeInBytes())
		cr.dataBytesByMtype[mtype] += uint64(context.DataSizeInBytes())
	}

	return contextKey
}

func (cr *contextResolver) get(key ckey.ContextKey) (*Context, bool) {
	ctx, found := cr.contextsByKey[key]
	return ctx, found
}

func (cr *contextResolver) length() int {
	return len(cr.contextsByKey)
}

func (cr *contextResolver) remove(expiredContextKey ckey.ContextKey) {
	panic("not called")
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
	panic("not called")
}

//nolint:revive // TODO(AML) Fix revive linter
func (cr *contextResolver) sendOriginTelemetry(timestamp float64, series metrics.SerieSink, hostname string, constTags []string) {
	panic("not called")
}

// timestampContextResolver allows tracking and expiring contexts based on time.
type timestampContextResolver struct {
	resolver      *contextResolver
	lastSeenByKey map[ckey.ContextKey]float64
}

func newTimestampContextResolver(cache *tags.Store, id string) *timestampContextResolver {
	return &timestampContextResolver{
		resolver:      newContextResolver(cache, id),
		lastSeenByKey: make(map[ckey.ContextKey]float64),
	}
}

// updateTrackedContext updates the last seen timestamp on a given context key
func (cr *timestampContextResolver) updateTrackedContext(contextKey ckey.ContextKey, timestamp float64) error {
	panic("not called")
}

// trackContext returns the contextKey associated with the context of the metricSample and tracks that context
func (cr *timestampContextResolver) trackContext(metricSampleContext metrics.MetricSampleContext, currentTimestamp float64) ckey.ContextKey {
	contextKey := cr.resolver.trackContext(metricSampleContext)
	cr.lastSeenByKey[contextKey] = currentTimestamp
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
// and returns the associated contextKeys.
// keep can be used to retain contexts longer than their natural expiration time based on some condition.
func (cr *timestampContextResolver) expireContexts(expireTimestamp float64, keep func(ckey.ContextKey) bool) {
	for contextKey, lastSeen := range cr.lastSeenByKey {
		if lastSeen < expireTimestamp && (keep == nil || !keep(contextKey)) {
			delete(cr.lastSeenByKey, contextKey)
			cr.resolver.remove(contextKey)
		}
	}
}

func (cr *timestampContextResolver) sendOriginTelemetry(timestamp float64, series metrics.SerieSink, hostname string, tags []string) {
	panic("not called")
}

func (cr *timestampContextResolver) dumpContexts(dest io.Writer) error {
	panic("not called")
}

func (cr *timestampContextResolver) updateMetrics(countsByMTypeGauge telemetry.Gauge, bytesByMTypeGauge telemetry.Gauge) {
	cr.resolver.updateMetrics(countsByMTypeGauge, bytesByMTypeGauge)
}

// countBasedContextResolver allows tracking and expiring contexts based on the number
// of calls of `expireContexts`.
type countBasedContextResolver struct {
	resolver            *contextResolver
	expireCountByKey    map[ckey.ContextKey]int64
	expireCount         int64
	expireCountInterval int64
}

func newCountBasedContextResolver(expireCountInterval int, cache *tags.Store, id string) *countBasedContextResolver {
	panic("not called")
}

// length returns the number of contexts tracked by the resolver
func (cr *countBasedContextResolver) length() int {
	panic("not called")
}

func (cr *countBasedContextResolver) updateMetrics(countsByMTypeGauge telemetry.Gauge, bytesByMTypeGauge telemetry.Gauge) {
	panic("not called")
}

// trackContext returns the contextKey associated with the context of the metricSample and tracks that context
func (cr *countBasedContextResolver) trackContext(metricSampleContext metrics.MetricSampleContext) ckey.ContextKey {
	panic("not called")
}

func (cr *countBasedContextResolver) get(key ckey.ContextKey) (*Context, bool) {
	panic("not called")
}

// expireContexts cleans up the contexts that haven't been tracked since `expirationCount`
// call to `expireContexts` and returns the associated contextKeys
func (cr *countBasedContextResolver) expireContexts() []ckey.ContextKey {
	panic("not called")
}

func (cr *countBasedContextResolver) release() {
	panic("not called")
}
