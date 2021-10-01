package context_resolver

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util"
)

// ContextResolver allows tracking and expiring contexts
type contextResolverInMemory struct {
	contextResolverBase
	contextsByKey map[ckey.ContextKey]*Context
}

// NewContextResolverInMemory creates a new context resolver storing everything in memory
func NewContextResolverInMemory() *contextResolverInMemory {
	return &contextResolverInMemory{
		contextResolverBase: contextResolverBase{
			keyGenerator: ckey.NewKeyGenerator(),
			tagsBuffer:   util.NewTagsBuilder(),
		},
		contextsByKey: make(map[ckey.ContextKey]*Context),
	}
}

// TrackContext returns the contextKey associated with the context of the metricSample and tracks that context
func (cr *contextResolverInMemory) TrackContext(metricSampleContext metrics.MetricSampleContext) ckey.ContextKey {
	metricSampleContext.GetTags(cr.tagsBuffer)               // tags here are not sorted and can contain duplicates
	contextKey := cr.generateContextKey(metricSampleContext) // the generator will remove duplicates from cr.tagsBuffer (and doesn't mind the order)

	if _, ok := cr.contextsByKey[contextKey]; !ok {
		// making a copy of tags for the context since tagsBuffer
		// will be reused later. This allow us to allocate one slice
		// per context instead of one per sample.
		context := &Context{
			Name: metricSampleContext.GetName(),
			Tags: cr.tagsBuffer.Copy(),
			Host: metricSampleContext.GetHost(),
		}
		cr.Add(contextKey, context)
	}

	cr.tagsBuffer.Reset()
	return contextKey
}

func (cr *contextResolverInMemory) Add(key ckey.ContextKey, context *Context) {
	cr.contextsByKey[key] = context
}

// Get gets a context from its key
func (cr *contextResolverInMemory) Get(key ckey.ContextKey) (*Context, bool) {
	ctx, found := cr.contextsByKey[key]
	return ctx, found
}

// Size return the number of objects in the resolver
func (cr *contextResolverInMemory) Size() int {
	return len(cr.contextsByKey)
}

func (cr *contextResolverInMemory) removeKeys(expiredContextKeys []ckey.ContextKey) {
	for _, expiredContextKey := range expiredContextKeys {
		delete(cr.contextsByKey, expiredContextKey)
	}
}

// Clear drops all contexts
func (cr *contextResolverInMemory) Clear() {
	cr.contextsByKey = make(map[ckey.ContextKey]*Context)
}

// Close frees up resources
func (cr *contextResolverInMemory) Close() {
	cr.Clear()
}
