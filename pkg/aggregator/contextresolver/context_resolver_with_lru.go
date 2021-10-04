package contextresolver

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/golang/groupcache/lru"
)

// WithLRU allows tracking and expiring contexts
type WithLRU struct {
	contextResolverBase
	cache    *LRU
	resolver ContextResolver
}

// NewWithLRU creates a new ContextResolver using an LRU.
func NewWithLRU(resolver ContextResolver, cacheSize int) *WithLRU {
	cache := NewContextResolverLRU(cacheSize)
	cache.cache.OnEvicted = func(key lru.Key, value interface{}) {
		v := value.(*Context)
		k := key.(ckey.ContextKey)
		resolver.Add(k, v)
	}
	return &WithLRU{
		contextResolverBase: newContextResolverBase(),
		cache:               cache,
		resolver:            resolver,
	}
}

// TrackContext returns the contextKey associated with the context of the metricSample and tracks that context
func (cr *WithLRU) TrackContext(metricSampleContext metrics.MetricSampleContext) ckey.ContextKey {
	// There is room for optimization here are both methods are doing similar things.
	return cr.cache.TrackContext(metricSampleContext)
}

// Add tracks a context key in the ContextResolver.
func (cr *WithLRU) Add(key ckey.ContextKey, context *Context) {
	cr.cache.Add(key, context)
}

// Get gets a context from its key
func (cr *WithLRU) Get(key ckey.ContextKey) (*Context, bool) {
	ctx, found := cr.cache.Get(key)
	if !found {
		ctx, found = cr.resolver.Get(key)
	}
	return ctx, found
}

// Size returns the number of contexts in the resolver
func (cr *WithLRU) Size() int {
	return cr.resolver.Size()
}

func (cr *WithLRU) removeKeys(expiredContextKeys []ckey.ContextKey) {
	cr.cache.removeKeys(expiredContextKeys)
	cr.resolver.removeKeys(expiredContextKeys)
}

// Clear clears the context resolver data, dropping all contexts.
func (cr *WithLRU) Clear() {
	cr.cache.Clear()
	cr.resolver.Clear()
}

// Close frees resources used by the context resolver.
func (cr *WithLRU) Close() {
	cr.cache.Close()
	cr.resolver.Close()
}
