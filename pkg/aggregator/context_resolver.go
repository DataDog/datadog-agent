package aggregator

import (
	// stdlib
	"sort"
	"strings"

	// 3p
	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// Context holds the elements that form a context, and can be serialized into a context key
type Context struct {
	Name string
	Tags []string
	Host string
}

// ContextResolver allows tracking and expiring contexts
type ContextResolver struct {
	contextsByKey map[string]*Context
	lastSeenByKey map[string]float64
}

// generateContextKey generates the contextKey associated with the context of the metricSample
func generateContextKey(metricSample *metrics.MetricSample) string {
	var contextFields []string

	contextFields = append(contextFields, metricSample.Name)
	sort.Strings(metricSample.Tags)
	contextFields = append(contextFields, metricSample.Tags...)
	contextFields = append(contextFields, metricSample.Host)

	return strings.Join(contextFields, ",")
}

func newContextResolver() *ContextResolver {
	return &ContextResolver{
		contextsByKey: make(map[string]*Context),
		lastSeenByKey: make(map[string]float64),
	}
}

// trackContext returns the contextKey associated with the context of the metricSample and tracks that context
func (cr *ContextResolver) trackContext(metricSample *metrics.MetricSample, currentTimestamp float64) string {
	contextKey := generateContextKey(metricSample)
	if _, ok := cr.contextsByKey[contextKey]; !ok {
		cr.contextsByKey[contextKey] = &Context{
			Name: metricSample.Name,
			Tags: metricSample.Tags,
			Host: metricSample.Host,
		}
	}
	cr.lastSeenByKey[contextKey] = currentTimestamp

	return contextKey
}

// expireContexts cleans up the contexts that haven't been tracked since the given timestamp
// and returns the associated contextKeys
func (cr *ContextResolver) expireContexts(expireTimestamp float64) []string {
	var expiredContextKeys []string

	// Find expired context keys
	for contextKey, lastSeen := range cr.lastSeenByKey {
		if lastSeen < expireTimestamp {
			expiredContextKeys = append(expiredContextKeys, contextKey)
			log.Debugf("Context key '%s' expired", contextKey)
		}
	}

	// Delete expired context keys
	for _, expiredContextKey := range expiredContextKeys {
		delete(cr.contextsByKey, expiredContextKey)
		delete(cr.lastSeenByKey, expiredContextKey)
	}

	return expiredContextKeys
}
