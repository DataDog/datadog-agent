package contextresolver

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util"

	"github.com/golang/groupcache/lru"
)

// LRU allows tracking and expiring contexts
type LRU struct {
	contextResolverBase
	cache *lru.Cache
}

// NewContextResolverLRU returns a new ContextResolver using a LRU.
func NewContextResolverLRU(cacheSize int) *LRU {
	return &LRU{
		contextResolverBase: contextResolverBase{
			keyGenerator: ckey.NewKeyGenerator(),
			tagsBuffer:   util.NewHashingTagsBuilder(),
		},
		cache: lru.New(cacheSize),
	}
}

// TrackContext returns the contextKey associated with the context of the metricSample and tracks that context
func (cr *LRU) TrackContext(metricSampleContext metrics.MetricSampleContext) ckey.ContextKey {
	metricSampleContext.GetTags(cr.tagsBuffer)               // tags here are not sorted and can contain duplicates
	contextKey := cr.generateContextKey(metricSampleContext) // the generator will remove duplicates from cr.tagsBuffer (and doesn't mind the order)

	if _, ok := cr.cache.Get(contextKey); !ok {
		// making a copy of tags for the context since tagsBuffer
		// will be reused later. This allows us to allocate one slice
		// per context instead of one per sample.
		context := &Context{
			Name: metricSampleContext.GetName(),
			Tags: cr.tagsBuffer.Copy(),
			Host: metricSampleContext.GetHost(),
		}
		cr.cache.Add(contextKey, context)
	}

	cr.tagsBuffer.Reset()
	return contextKey
}

// Add adds a context key to this ContextResolver
func (cr *LRU) Add(key ckey.ContextKey, context *Context) {
	cr.cache.Add(key, context)
}

// Get gets a context matching a key
func (cr *LRU) Get(key ckey.ContextKey) (*Context, bool) {
	ctx, found := cr.cache.Get(key)
	return ctx.(*Context), found
}

// Size returns the number of objects in the cache
func (cr *LRU) Size() int {
	return cr.cache.Len()
}

func (cr *LRU) removeKeys(expiredContextKeys []ckey.ContextKey) {
	for _, expiredContextKey := range expiredContextKeys {
		cr.cache.Remove(expiredContextKey)
	}
}

// Clear drops all contexts
func (cr *LRU) Clear() {
	cr.cache.Clear()
}

// Close frees up resources
func (cr *LRU) Close() {
	cr.cache.Clear()
}
