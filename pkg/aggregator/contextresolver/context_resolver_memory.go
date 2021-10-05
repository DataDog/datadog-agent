package contextresolver

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util"
)

// InMemory allows tracking and expiring contexts
type InMemory struct {
	contextResolverBase
	contextsByKey map[ckey.ContextKey]*Context
}

// NewInMemory creates a new context resolver storing everything in memory
func NewInMemory() *InMemory {
	return &InMemory{
		contextResolverBase: contextResolverBase{
			keyGenerator: ckey.NewKeyGenerator(),
			tagsBuffer:   util.NewHashingTagsBuilder(),
		},
		contextsByKey: make(map[ckey.ContextKey]*Context),
	}
}

// TrackContext returns the contextKey associated with the context of the metricSample and tracks that context
func (cr *InMemory) TrackContext(metricSampleContext metrics.MetricSampleContext) ckey.ContextKey {
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

// Add tracks a context key in the ContextResolver.
func (cr *InMemory) Add(key ckey.ContextKey, context *Context) {
	cr.contextsByKey[key] = context
}

// Get gets a context from its key
func (cr *InMemory) Get(key ckey.ContextKey) (*Context, bool) {
	ctx, found := cr.contextsByKey[key]
	return ctx, found
}

// Size return the number of objects in the resolver
func (cr *InMemory) Size() int {
	return len(cr.contextsByKey)
}

func (cr *InMemory) removeKeys(expiredContextKeys []ckey.ContextKey) {
	for _, expiredContextKey := range expiredContextKeys {
		delete(cr.contextsByKey, expiredContextKey)
	}
}

// Clear clears the context resolver data, dropping all contexts.
func (cr *InMemory) Clear() {
	cr.contextsByKey = make(map[ckey.ContextKey]*Context)
}

// Close frees resources used by the context resolver.
func (cr *InMemory) Close() {
	cr.Clear()
}
