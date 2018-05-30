// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package aggregator

import (
	// stdlib
	"fmt"

	// 3p
	"github.com/DataDog/datadog-agent/pkg/util/log"

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
func generateContextKey(metricSample *metrics.MetricSample) ckey.ContextKey {
	return ckey.Generate(metricSample.Name, metricSample.Host, metricSample.Tags)
}

func newContextResolver() *ContextResolver {
	return &ContextResolver{
		contextsByKey: make(map[ckey.ContextKey]*Context),
		lastSeenByKey: make(map[ckey.ContextKey]float64),
	}
}

// trackContext returns the contextKey associated with the context of the metricSample and tracks that context
func (cr *ContextResolver) trackContext(metricSample *metrics.MetricSample, currentTimestamp float64) ckey.ContextKey {
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
