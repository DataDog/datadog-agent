package contextresolver

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// TimestampContextResolver allows tracking and expiring contexts based on time.
type TimestampContextResolver struct {
	resolver      ContextResolver
	lastSeenByKey map[ckey.ContextKey]float64
}

// NewTimestampContextResolver returns a new ContextResolver based on timestamp.
func NewTimestampContextResolver() *TimestampContextResolver {
	return &TimestampContextResolver{
		resolver:      newContextResolver(),
		lastSeenByKey: make(map[ckey.ContextKey]float64),
	}
}

// UpdateTrackedContext updates the last seen timestamp on a given context key
func (cr *TimestampContextResolver) UpdateTrackedContext(contextKey ckey.ContextKey, timestamp float64) error {
	if _, ok := cr.lastSeenByKey[contextKey]; ok && cr.lastSeenByKey[contextKey] < timestamp {
		cr.lastSeenByKey[contextKey] = timestamp
	} else if !ok {
		return fmt.Errorf("Trying to update a context that is not tracked")
	}

	return nil
}

// TrackContext returns the contextKey associated with the context of the metricSample and tracks that context
func (cr *TimestampContextResolver) TrackContext(metricSampleContext metrics.MetricSampleContext, currentTimestamp float64) ckey.ContextKey {
	contextKey := cr.resolver.TrackContext(metricSampleContext)
	cr.lastSeenByKey[contextKey] = currentTimestamp
	return contextKey
}

// Size returns the number of contexts in the cache
func (cr *TimestampContextResolver) Size() int {
	return cr.resolver.Size()
}

// Get returns a context from its key
func (cr *TimestampContextResolver) Get(key ckey.ContextKey) (*Context, bool) {
	return cr.resolver.Get(key)
}

// ExpireContexts cleans up the contexts that haven't been tracked since the given timestamp
// and returns the associated contextKeys
func (cr *TimestampContextResolver) ExpireContexts(expireTimestamp float64) []ckey.ContextKey {
	var expiredContextKeys []ckey.ContextKey

	// Find expired context keys
	for contextKey, lastSeen := range cr.lastSeenByKey {
		if lastSeen < expireTimestamp {
			expiredContextKeys = append(expiredContextKeys, contextKey)
		}
	}

	cr.removeKeys(expiredContextKeys)

	// Delete expired context keys
	for _, expiredContextKey := range expiredContextKeys {
		delete(cr.lastSeenByKey, expiredContextKey)
	}

	return expiredContextKeys
}

// func (cr *TimestampContextResolver) generateContextKey(metricSampleContext metrics.MetricSampleContext) ckey.ContextKey {
// 	return cr.resolver.generateContextKey(metricSampleContext)
// }

func (cr *TimestampContextResolver) removeKeys(expiredContextKeys []ckey.ContextKey) {
	cr.resolver.removeKeys(expiredContextKeys)
}

// Clear frees up resources
func (cr *TimestampContextResolver) Clear() {
	cr.lastSeenByKey = make(map[ckey.ContextKey]float64)
	cr.resolver.Clear()
}

// Close frees up resources
func (cr *TimestampContextResolver) Close() {
	cr.resolver.Close()
}
