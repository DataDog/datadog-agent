package context_resolver

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// CountBasedContextResolver allows tracking and expiring contexts based on the number
// of calls of `expireContexts`.
type CountBasedContextResolver struct {
	resolver         ContextResolver
	expireCountByKey map[ckey.ContextKey]int64
	expireCount         int64
	expireCountInterval int64
}

// NewCountBasedContextResolver creates a new count based resolver
func NewCountBasedContextResolver(expireCountInterval int) *CountBasedContextResolver {
	return &CountBasedContextResolver{
		resolver:            newContextResolver(),
		expireCountByKey:    make(map[ckey.ContextKey]int64),
		expireCount:         0,
		expireCountInterval: int64(expireCountInterval),
	}
}

// TrackContext returns the contextKey associated with the context of the metricSample and tracks that context
func (cr *CountBasedContextResolver) TrackContext(metricSampleContext metrics.MetricSampleContext) ckey.ContextKey {
	contextKey := cr.resolver.TrackContext(metricSampleContext)
	cr.expireCountByKey[contextKey] = cr.expireCount
	return contextKey
}

func (cr *CountBasedContextResolver) Get(key ckey.ContextKey) (*Context, bool) {
	return cr.resolver.Get(key)
}

// ExpireContexts cleans up the contexts that haven't been tracked since `expirationCount`
// call to `expireContexts` and returns the associated contextKeys
func (cr *CountBasedContextResolver) ExpireContexts() []ckey.ContextKey {
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

func (cr *CountBasedContextResolver) generateContextKey(metricSampleContext metrics.MetricSampleContext) ckey.ContextKey {
	return cr.resolver.generateContextKey(metricSampleContext)
}

func (cr *CountBasedContextResolver) removeKeys(expiredContextKeys []ckey.ContextKey) {
	cr.resolver.removeKeys(expiredContextKeys)
}

// Size returns the number of objects
func (cr *CountBasedContextResolver) Size() int {
	return cr.resolver.Size()
}

// Clear drops all contexts
func (cr *CountBasedContextResolver) Clear() {
	cr.expireCountByKey = make(map[ckey.ContextKey]int64)
	cr.resolver.Clear()
}

// Close closes the resolver and free up resources
func (cr *CountBasedContextResolver) Close() {
	cr.Clear()
}
