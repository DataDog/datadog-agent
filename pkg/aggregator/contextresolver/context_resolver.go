// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package contextresolver

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util"
)

// Context holds the elements that form a context, and can be serialized into a context key
type Context struct {
	Name string
	Tags []string
	Host string
}

// FIXME: make this an option.
const Resolver = "dedup"

// ContextResolver allows tracking and expiring contexts
type ContextResolver interface {
	generateContextKey(metricSampleContext metrics.MetricSampleContext) ckey.ContextKey
	TrackContext(metricSampleContext metrics.MetricSampleContext) ckey.ContextKey
	Add(key ckey.ContextKey, context *Context)
	Get(key ckey.ContextKey) (*Context, bool)
	Size() int
	removeKeys(expiredContextKeys []ckey.ContextKey)
	Clear()
	Close()
}

func newContextResolverBase() contextResolverBase {
	return contextResolverBase{
		keyGenerator: ckey.NewKeyGenerator(),
		tagsBuffer:   util.NewHashingTagsBuilder(),
	}
}

func newContextResolver() ContextResolver {
	if Resolver == "dedup" {
		// FIXME: add options to be able to use files.
		return NewDedup()
	} else if Resolver == "badger" {
		return NewBadger(true, "")
	} else if Resolver == "badger-disk" {
		// FIXME
		return NewBadger(true, "")
	}
	return NewInMemory()
}

type contextResolverBase struct {
	keyGenerator *ckey.KeyGenerator
	// buffer slice allocated once per ContextResolver to combine and sort
	// tags, origin detection tags and k8s tags.
	tagsBuffer *util.HashingTagsBuilder
}

// generateContextKey generates the contextKey associated with the context of the metricSample
func (cr *contextResolverBase) generateContextKey(metricSampleContext metrics.MetricSampleContext) ckey.ContextKey {
	return cr.keyGenerator.Generate(metricSampleContext.GetName(), metricSampleContext.GetHost(), cr.tagsBuffer)
}
