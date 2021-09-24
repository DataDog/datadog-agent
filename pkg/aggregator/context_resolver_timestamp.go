package aggregator

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// timestampContextResolver allows tracking and expiring contexts based on time.
type timestampContextResolver struct {
	resolver      contextResolver
	lastSeenByKey map[ckey.ContextKey]float64
}

func newTimestampContextResolver() *timestampContextResolver {
	return &timestampContextResolver{
		resolver:      newContextResolver(),
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

func (cr *timestampContextResolver) get(key ckey.ContextKey) (*Context, bool) {
	return cr.resolver.get(key)
}

// expireContexts cleans up the contexts that haven't been tracked since the given timestamp
// and returns the associated contextKeys
func (cr *timestampContextResolver) expireContexts(expireTimestamp float64) []ckey.ContextKey {
	var expiredContextKeys []ckey.ContextKey

	// Find expired context keys
	for contextKey, lastSeen := range cr.lastSeenByKey {
		if lastSeen < expireTimestamp {
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

func (cr *timestampContextResolver) close() {
	cr.resolver.close()
}
