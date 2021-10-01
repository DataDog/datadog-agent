package context_resolver

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util"

	"github.com/golang/groupcache/lru"
)

// ContextResolver allows tracking and expiring contexts
type contextResolverLru struct {
	contextResolverBase
	cache *lru.Cache
}

func NewContextResolverLru(cacheSize int) *contextResolverLru {
	return &contextResolverLru{
		contextResolverBase: contextResolverBase{
			keyGenerator: ckey.NewKeyGenerator(),
			tagsBuffer:   util.NewTagsBuilder(),
		},
		cache: lru.New(cacheSize),
	}
}

// TrackContext returns the contextKey associated with the context of the metricSample and tracks that context
func (cr *contextResolverLru) TrackContext(metricSampleContext metrics.MetricSampleContext) ckey.ContextKey {
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

func (cr *contextResolverLru) Add(key ckey.ContextKey, context *Context) {
	cr.cache.Add(key, context)
}

// Get gets a context matching a key
func (cr *contextResolverLru) Get(key ckey.ContextKey) (*Context, bool) {
	ctx, found := cr.cache.Get(key)
	return ctx.(*Context), found
}

// Size returns the number of objects in the cache
func (cr *contextResolverLru) Size() int {
	return cr.cache.Len()
}

func (cr *contextResolverLru) removeKeys(expiredContextKeys []ckey.ContextKey) {
	for _, expiredContextKey := range expiredContextKeys {
		cr.cache.Remove(expiredContextKey)
	}
}

// Clear drops all contexts
func (cr *contextResolverLru) Clear() {
	cr.cache.Clear()
}

// Close frees up resources
func (cr *contextResolverLru) Close() {
	cr.cache.Clear()
}
