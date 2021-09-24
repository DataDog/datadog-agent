// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// Context holds the elements that form a context, and can be serialized into a context key
type Context struct {
	Name string
	Tags []string
	Host string
}

// FIXME: make this an option.
const UseBadger = true

// contextResolver allows tracking and expiring contexts
type contextResolver interface {
	generateContextKey(metricSampleContext metrics.MetricSampleContext) ckey.ContextKey
	trackContext(metricSampleContext metrics.MetricSampleContext) ckey.ContextKey
	get(key ckey.ContextKey) (*Context, bool)
	length() int
	removeKeys(expiredContextKeys []ckey.ContextKey)
	close()
}

func newContextResolver() contextResolver {
	if UseBadger {
		return newContextResolverBadger()
	}
	return newContextResolverInMemory()
}
