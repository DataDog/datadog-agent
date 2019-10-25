// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

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
}

// generateContextKey generates the contextKey associated with the context of the metricSample
func generateContextKey(metricSampleContext metrics.MetricSampleContext) ckey.ContextKey {
	return ckey.Generate(metricSampleContext.GetName(), metricSampleContext.GetHost(), metricSampleContext.GetTags())
}

// generateContextKey generates the contextKey associated with the context of the metricSample
func generateContextKeyBytes(name []byte, tags [][]byte, hostname []byte) ckey.ContextKey {
	return ckey.GenerateBytes(name, hostname, tags)
}

func newContextResolver() *ContextResolver {
	return &ContextResolver{
		contextsByKey: make(map[ckey.ContextKey]*Context),
		lastSeenByKey: make(map[ckey.ContextKey]float64),
	}
}

func convertBytesTags(rawTags [][]byte) []string {
	tags := make([]string, 0, len(rawTags))
	for _, tag := range rawTags {
		tags = append(tags, string(tag))
	}
	return tags
}

func (cr *ContextResolver) trackContextBytes(name []byte, tags [][]byte, hostname []byte, currentTimestamp float64) ckey.ContextKey {
	contextKey := generateContextKeyBytes(name, tags, hostname)
	if _, ok := cr.contextsByKey[contextKey]; !ok {
		cr.contextsByKey[contextKey] = &Context{
			Name: string(name),
			Tags: convertBytesTags(tags),
			Host: string(hostname),
		}
	}
	cr.lastSeenByKey[contextKey] = currentTimestamp

	return contextKey
}

// trackContext returns the contextKey associated with the context of the metricSample and tracks that context
func (cr *ContextResolver) trackContext(metricSampleContext metrics.MetricSampleContext, currentTimestamp float64) ckey.ContextKey {
	contextKey := generateContextKey(metricSampleContext)
	if _, ok := cr.contextsByKey[contextKey]; !ok {
		cr.contextsByKey[contextKey] = &Context{
			Name: metricSampleContext.GetName(),
			Tags: metricSampleContext.GetTags(),
			Host: metricSampleContext.GetHost(),
		}
	}
	cr.lastSeenByKey[contextKey] = currentTimestamp

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
