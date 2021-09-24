package aggregator

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util"
)

// contextResolver allows tracking and expiring contexts
type contextResolverInMemory struct {
	contextsByKey map[ckey.ContextKey]*Context
	keyGenerator  *ckey.KeyGenerator
	// buffer slice allocated once per contextResolver to combine and sort
	// tags, origin detection tags and k8s tags.
	tagsBuffer *util.TagsBuilder
}

// generateContextKey generates the contextKey associated with the context of the metricSample
func (cr *contextResolverInMemory) generateContextKey(metricSampleContext metrics.MetricSampleContext) ckey.ContextKey {
	return cr.keyGenerator.Generate(metricSampleContext.GetName(), metricSampleContext.GetHost(), cr.tagsBuffer)
}

func newContextResolverInMemory() *contextResolverInMemory {
	return &contextResolverInMemory{
		contextsByKey: make(map[ckey.ContextKey]*Context),
		keyGenerator:  ckey.NewKeyGenerator(),
		tagsBuffer:    util.NewTagsBuilder(),
	}
}

// trackContext returns the contextKey associated with the context of the metricSample and tracks that context
func (cr *contextResolverInMemory) trackContext(metricSampleContext metrics.MetricSampleContext) ckey.ContextKey {
	metricSampleContext.GetTags(cr.tagsBuffer)               // tags here are not sorted and can contain duplicates
	contextKey := cr.generateContextKey(metricSampleContext) // the generator will remove duplicates from cr.tagsBuffer (and doesn't mind the order)

	if _, ok := cr.contextsByKey[contextKey]; !ok {
		// making a copy of tags for the context since tagsBuffer
		// will be reused later. This allow us to allocate one slice
		// per context instead of one per sample.
		cr.contextsByKey[contextKey] = &Context{
			Name: metricSampleContext.GetName(),
			Tags: cr.tagsBuffer.Copy(),
			Host: metricSampleContext.GetHost(),
		}
	}

	cr.tagsBuffer.Reset()
	return contextKey
}

func (cr *contextResolverInMemory) get(key ckey.ContextKey) (*Context, bool) {
	ctx, found := cr.contextsByKey[key]
	return ctx, found
}

func (cr *contextResolverInMemory) length() int {
	return len(cr.contextsByKey)
}

func (cr *contextResolverInMemory) removeKeys(expiredContextKeys []ckey.ContextKey) {
	for _, expiredContextKey := range expiredContextKeys {
		delete(cr.contextsByKey, expiredContextKey)
	}
}

func (cr *contextResolverInMemory) close() {
}
