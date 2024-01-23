// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/tags"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

const checksSourceTypeName = "System"

// CheckSampler aggregates metrics from one Check instance
type CheckSampler struct {
	id                     checkid.ID
	series                 []*metrics.Serie
	sketches               metrics.SketchSeriesList
	contextResolver        *countBasedContextResolver
	metrics                metrics.CheckMetrics
	sketchMap              sketchMap
	lastBucketValue        map[ckey.ContextKey]int64
	deregistered           bool
	contextResolverMetrics bool
}

// newCheckSampler returns a newly initialized CheckSampler
func newCheckSampler(expirationCount int, expireMetrics bool, contextResolverMetrics bool, statefulTimeout time.Duration, cache *tags.Store, id checkid.ID) *CheckSampler {
	panic("not called")
}

func (cs *CheckSampler) addSample(metricSample *metrics.MetricSample) {
	panic("not called")
}

func (cs *CheckSampler) newSketchSeries(ck ckey.ContextKey, points []metrics.SketchPoint) *metrics.SketchSeries {
	panic("not called")
}

func (cs *CheckSampler) addBucket(bucket *metrics.HistogramBucket) {
	panic("not called")
}

func (cs *CheckSampler) commitSeries(timestamp float64) {
	panic("not called")
}

func (cs *CheckSampler) commitSketches(timestamp float64) {
	panic("not called")
}

func (cs *CheckSampler) commit(timestamp float64) {
	panic("not called")
}

func (cs *CheckSampler) flush() (metrics.Series, metrics.SketchSeriesList) {
	panic("not called")
}

func (cs *CheckSampler) release() {
	panic("not called")
}

func (cs *CheckSampler) releaseMetrics() {
	panic("not called")
}

func (cs *CheckSampler) updateMetrics() {
	panic("not called")
}
