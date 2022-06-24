// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/tags"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// Context holds the elements that form a context, and can be serialized into a context key
type Context struct {
	Name       string
	Host       string
	mtype      metrics.MetricType
	taggerTags *tags.Entry
	metricTags *tags.Entry
}

// Tags returns tags for the context.
func (c *Context) Tags() tagset.CompositeTags {
	return tagset.NewCompositeTags(c.taggerTags.Tags(), c.metricTags.Tags())
}

func (c *Context) release() {
	c.taggerTags.Release()
	c.metricTags.Release()
}

// contextResolver allows tracking and expiring contexts
type contextResolver struct {
	contextsByKey map[ckey.ContextKey]*Context
	countsByMtype []uint64
	tagsCache     *tags.Store
	keyGenerator  *ckey.KeyGenerator
	taggerBuffer  *tagset.HashingTagsAccumulator
	metricBuffer  *tagset.HashingTagsAccumulator
}

// generateContextKey generates the contextKey associated with the context of the metricSample
func (cr *contextResolver) generateContextKey(metricSampleContext metrics.MetricSampleContext) (ckey.ContextKey, ckey.TagsKey, ckey.TagsKey) {
	return cr.keyGenerator.GenerateWithTags2(metricSampleContext.GetName(), metricSampleContext.GetHost(), cr.taggerBuffer, cr.metricBuffer)
}

func newContextResolver(cache *tags.Store) *contextResolver {
	return &contextResolver{
		contextsByKey: make(map[ckey.ContextKey]*Context),
		countsByMtype: make([]uint64, metrics.NumMetricTypes),
		tagsCache:     cache,
		keyGenerator:  ckey.NewKeyGenerator(),
		taggerBuffer:  tagset.NewHashingTagsAccumulator(),
		metricBuffer:  tagset.NewHashingTagsAccumulator(),
	}
}

// trackContext returns the contextKey associated with the context of the metricSample and tracks that context
func (cr *contextResolver) trackContext(metricSampleContext metrics.MetricSampleContext) ckey.ContextKey {
	metricSampleContext.GetTags(cr.taggerBuffer, cr.metricBuffer)                  // tags here are not sorted and can contain duplicates
	contextKey, taggerKey, metricKey := cr.generateContextKey(metricSampleContext) // the generator will remove duplicates (and doesn't mind the order)

	if _, ok := cr.contextsByKey[contextKey]; !ok {
		mtype := metricSampleContext.GetMetricType()
		cr.contextsByKey[contextKey] = &Context{
			Name:       metricSampleContext.GetName(),
			taggerTags: cr.tagsCache.Insert(taggerKey, cr.taggerBuffer),
			metricTags: cr.tagsCache.Insert(metricKey, cr.metricBuffer),
			Host:       metricSampleContext.GetHost(),
			mtype:      mtype,
		}
		cr.countsByMtype[mtype]++
	}

	cr.taggerBuffer.Reset()
	cr.metricBuffer.Reset()

	return contextKey
}

func (cr *contextResolver) get(key ckey.ContextKey) (*Context, bool) {
	ctx, found := cr.contextsByKey[key]
	return ctx, found
}

func (cr *contextResolver) length() int {
	return len(cr.contextsByKey)
}

func (cr *contextResolver) removeKeys(expiredContextKeys []ckey.ContextKey) {
	for _, expiredContextKey := range expiredContextKeys {
		context := cr.contextsByKey[expiredContextKey]
		delete(cr.contextsByKey, expiredContextKey)

		if context != nil {
			cr.countsByMtype[context.mtype]--
			context.release()
		}
	}
}

func (cr *contextResolver) release() {
	for _, c := range cr.contextsByKey {
		c.release()
	}
}

// timestampContextResolver allows tracking and expiring contexts based on time.
type timestampContextResolver struct {
	resolver      *contextResolver
	lastSeenByKey map[ckey.ContextKey]float64
}

func newTimestampContextResolver(cache *tags.Store) *timestampContextResolver {
	return &timestampContextResolver{
		resolver:      newContextResolver(cache),
		lastSeenByKey: make(map[ckey.ContextKey]float64),
	}
}

// updateTrackedContext updates the last seen timestamp on a given context key
func (cr *timestampContextResolver) updateTrackedContext(contextKey ckey.ContextKey, timestamp float64) error {
	if _, ok := cr.lastSeenByKey[contextKey]; ok && cr.lastSeenByKey[contextKey] < timestamp {
		cr.lastSeenByKey[contextKey] = timestamp
	} else if !ok {
		return fmt.Errorf("Trying to update a context that is not tracked")
	}

	return nil
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
func (cr *timestampContextResolver) expireContexts(expireTimestamp float64, keep func(ckey.ContextKey) bool) []ckey.ContextKey {
	var expiredContextKeys []ckey.ContextKey

	// Find expired context keys
	for contextKey, lastSeen := range cr.lastSeenByKey {
		if lastSeen < expireTimestamp && (keep == nil || !keep(contextKey)) {
			expiredContextKeys = append(expiredContextKeys, contextKey)
		}
	}

	cr.resolver.removeKeys(expiredContextKeys)

	// Delete expired context keys
	for _, expiredContextKey := range expiredContextKeys {
		delete(cr.lastSeenByKey, expiredContextKey)
	}

	return expiredContextKeys
}

// countBasedContextResolver allows tracking and expiring contexts based on the number
// of calls of `expireContexts`.
type countBasedContextResolver struct {
	resolver            *contextResolver
	expireCountByKey    map[ckey.ContextKey]int64
	expireCount         int64
	expireCountInterval int64
}

func newCountBasedContextResolver(expireCountInterval int, cache *tags.Store) *countBasedContextResolver {
	return &countBasedContextResolver{
		resolver:            newContextResolver(cache),
		expireCountByKey:    make(map[ckey.ContextKey]int64),
		expireCount:         0,
		expireCountInterval: int64(expireCountInterval),
	}
}

// trackContext returns the contextKey associated with the context of the metricSample and tracks that context
func (cr *countBasedContextResolver) trackContext(metricSampleContext metrics.MetricSampleContext) ckey.ContextKey {
	contextKey := cr.resolver.trackContext(metricSampleContext)
	cr.expireCountByKey[contextKey] = cr.expireCount
	return contextKey
}

func (cr *countBasedContextResolver) get(key ckey.ContextKey) (*Context, bool) {
	return cr.resolver.get(key)
}

// expireContexts cleans up the contexts that haven't been tracked since `expirationCount`
// call to `expireContexts` and returns the associated contextKeys
func (cr *countBasedContextResolver) expireContexts() []ckey.ContextKey {
	var keys []ckey.ContextKey
	for key, index := range cr.expireCountByKey {
		if index <= cr.expireCount-cr.expireCountInterval {
			keys = append(keys, key)
			delete(cr.expireCountByKey, key)
		}
	}
	cr.resolver.removeKeys(keys)
	cr.expireCount++
	return keys
}

func (cr *countBasedContextResolver) release() {
	cr.resolver.release()
}
