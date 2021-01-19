// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package aggregator

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
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
	contextsByKey map[ckey.ContextKey]*Context
	lastSeenByKey map[ckey.ContextKey]float64
	keyGenerator  *ckey.KeyGenerator
	// buffer slice allocated once per ContextResolver to combine and sort
	// tags, origin detection tags and k8s tags.
	tagsSliceBuffer []string
}

// generateContextKey generates the contextKey associated with the context of the metricSample
func (cr *ContextResolver) generateContextKey(metricSampleContext metrics.MetricSampleContext, tags []string) ckey.ContextKey {
	return cr.keyGenerator.Generate(metricSampleContext.GetName(), metricSampleContext.GetHost(), tags)
}

func newContextResolver() *ContextResolver {
	return &ContextResolver{
		contextsByKey:   make(map[ckey.ContextKey]*Context),
		lastSeenByKey:   make(map[ckey.ContextKey]float64),
		keyGenerator:    ckey.NewKeyGenerator(),
		tagsSliceBuffer: make([]string, 0, 128),
	}
}

// trackContext returns the contextKey associated with the context of the metricSample and tracks that context
func (cr *ContextResolver) trackContext(metricSampleContext metrics.MetricSampleContext, currentTimestamp float64) ckey.ContextKey {
	cr.tagsSliceBuffer = metricSampleContext.GetTags(cr.tagsSliceBuffer)
	contextKey := cr.generateContextKey(metricSampleContext, cr.tagsSliceBuffer)

	if _, ok := cr.contextsByKey[contextKey]; !ok {
		// making a copy of tags for the context since tagsSliceBuffer
		// will be reused later. This allow us to allocate one slice
		// per context instead of one per sample.
		contextTags := append(make([]string, 0, len(cr.tagsSliceBuffer)), cr.tagsSliceBuffer...)

		cr.contextsByKey[contextKey] = &Context{
			Name: metricSampleContext.GetName(),
			Tags: contextTags,
			Host: metricSampleContext.GetHost(),
		}
	}
	cr.lastSeenByKey[contextKey] = currentTimestamp

	cr.tagsSliceBuffer = cr.tagsSliceBuffer[0:0] // reset tags buffer
	return contextKey
}

// updateTrackedContext updates the last seen timestamp on a given context key
func (cr *ContextResolver) updateTrackedContext(contextKey ckey.ContextKey, timestamp float64) error {
	if _, ok := cr.lastSeenByKey[contextKey]; ok && cr.lastSeenByKey[contextKey] < timestamp {
		cr.lastSeenByKey[contextKey] = timestamp
	} else if !ok {
		return fmt.Errorf("Trying to update a context that is not tracked")
	}

	return nil
}

// expireContexts cleans up the contexts that haven't been tracked since the given timestamp
// and returns the associated contextKeys
func (cr *ContextResolver) expireContexts(expireTimestamp float64) []ckey.ContextKey {
	var expiredContextKeys []ckey.ContextKey

	// Find expired context keys
	for contextKey, lastSeen := range cr.lastSeenByKey {
		if lastSeen < expireTimestamp {
			expiredContextKeys = append(expiredContextKeys, contextKey)
		}
	}

	// Delete expired context keys
	for _, expiredContextKey := range expiredContextKeys {
		delete(cr.contextsByKey, expiredContextKey)
		delete(cr.lastSeenByKey, expiredContextKey)
	}

	return expiredContextKeys
}
