// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package aggregator

import (
	"math"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const checksSourceTypeName = "System"

// CheckSampler aggregates metrics from one Check instance
type CheckSampler struct {
	series                   []*metrics.Serie
	sketches                 []metrics.SketchSeries
	contextResolver          *ContextResolver
	metrics                  metrics.ContextMetrics
	sketchMap                sketchMap
	lastBucketValue          map[ckey.ContextKey]int
	lastSeenBucket           map[ckey.ContextKey]time.Time
	bucketExpiry             time.Duration
	interpolationGranularity int
}

// newCheckSampler returns a newly initialized CheckSampler
func newCheckSampler() *CheckSampler {
	return &CheckSampler{
		series:                   make([]*metrics.Serie, 0),
		sketches:                 make([]metrics.SketchSeries, 0),
		contextResolver:          newContextResolver(),
		metrics:                  metrics.MakeContextMetrics(),
		sketchMap:                make(sketchMap),
		lastBucketValue:          make(map[ckey.ContextKey]int),
		lastSeenBucket:           make(map[ckey.ContextKey]time.Time),
		bucketExpiry:             1 * time.Minute,
		interpolationGranularity: 1000,
	}
}

func (cs *CheckSampler) addSample(metricSample *metrics.MetricSample) {
	contextKey := cs.contextResolver.trackContext(metricSample, metricSample.Timestamp)

	if err := cs.metrics.AddSample(contextKey, metricSample, metricSample.Timestamp, 1); err != nil {
		log.Debug("Ignoring sample '%s' on host '%s' and tags '%s': %s", metricSample.Name, metricSample.Host, metricSample.Tags, err)
	}
}

func (cs *CheckSampler) newSketchSeries(ck ckey.ContextKey, points []metrics.SketchPoint) metrics.SketchSeries {
	ctx := cs.contextResolver.contextsByKey[ck]
	ss := metrics.SketchSeries{
		Name: ctx.Name,
		Tags: ctx.Tags,
		Host: ctx.Host,
		// Interval: TODO: investigate
		Points:     points,
		ContextKey: ck,
	}

	return ss
}

func (cs *CheckSampler) addBucket(bucket *metrics.HistogramBucket) {
	if bucket.Value < 0 {
		log.Warnf("Negative bucket value %d for metric %s discarding", bucket.Value, bucket.Name)
		return
	}
	if bucket.Value == 0 {
		// noop
		return
	}

	bucketRange := bucket.UpperBound - bucket.LowerBound
	if bucketRange < 0 {
		log.Warnf(
			"Negative bucket range [%f-%f] for metric %s discarding",
			bucket.LowerBound, bucket.UpperBound, bucket.Name,
		)
		return
	}

	contextKey := cs.contextResolver.trackContext(bucket, bucket.Timestamp)

	// if the bucket is monotonic and we have already seen the bucket we only send the delta
	if bucket.Monotonic {
		lastBucketValue, bucketFound := cs.lastBucketValue[contextKey]
		rawValue := bucket.Value
		if bucketFound {
			cs.lastSeenBucket[contextKey] = time.Now()
			bucket.Value = rawValue - lastBucketValue
		}
		cs.lastBucketValue[contextKey] = rawValue
		cs.lastSeenBucket[contextKey] = time.Now()
	}

	if bucket.Value < 0 {
		log.Warnf("Negative bucket delta %d for metric %s discarding", bucket.Value, bucket.Name)
		return
	}
	if bucket.Value == 0 {
		// noop
		return
	}

	// simple linear interpolation, TODO: optimize
	var linearIncr float64
	var incrCount int
	var countPerIncr uint
	if bucket.Value > cs.interpolationGranularity {
		linearIncr = bucketRange / float64(cs.interpolationGranularity)
		countPerIncr = uint(bucket.Value / cs.interpolationGranularity)
		incrCount = cs.interpolationGranularity
	} else {
		linearIncr = bucketRange / float64(bucket.Value)
		countPerIncr = 1
		incrCount = bucket.Value
	}
	if math.IsInf(bucket.UpperBound, 1) {
		// We simulate the behavior of promQL for the infinity bucket:
		// "if the quantile falls into the highest bucket, the upper bound of the 2nd highest bucket is returned"
		incrCount = 1
		countPerIncr = uint(bucket.Value)
	}
	currentVal := bucket.LowerBound
	log.Tracef(
		"Interpolating %d values by group of %d over the [%f-%f] bucket with %f increment",
		bucket.Value, countPerIncr, bucket.LowerBound, bucket.UpperBound, linearIncr,
	)
	for i := 0; i < incrCount; i++ {
		cs.sketchMap.insertN(int64(bucket.Timestamp), contextKey, currentVal, countPerIncr)
		currentVal += linearIncr
	}
}

func (cs *CheckSampler) commitSeries(timestamp float64) {
	series, errors := cs.metrics.Flush(timestamp)
	for ckey, err := range errors {
		context, ok := cs.contextResolver.contextsByKey[ckey]
		if !ok {
			log.Errorf("Can't resolve context of error '%s': inconsistent context resolver state: context with key '%v' is not tracked", err, ckey)
			continue
		}
		log.Infof("No value returned for check metric '%s' on host '%s' and tags '%s': %s", context.Name, context.Host, context.Tags, err)
	}
	for _, serie := range series {
		// Resolve context and populate new []Serie
		context, ok := cs.contextResolver.contextsByKey[serie.ContextKey]
		if !ok {
			log.Errorf("Ignoring all metrics on context key '%v': inconsistent context resolver state: the context is not tracked", serie.ContextKey)
			continue
		}
		serie.Name = context.Name + serie.NameSuffix
		serie.Tags = context.Tags
		serie.Host = context.Host
		serie.SourceTypeName = checksSourceTypeName // this source type is required for metrics coming from the checks

		cs.series = append(cs.series, serie)
	}
}

func (cs *CheckSampler) commitSketches(timestamp float64) {
	pointsByCtx := make(map[ckey.ContextKey][]metrics.SketchPoint)

	cs.sketchMap.flushBefore(int64(timestamp), func(ck ckey.ContextKey, p metrics.SketchPoint) {
		if p.Sketch == nil {
			return
		}
		pointsByCtx[ck] = append(pointsByCtx[ck], p)
	})
	for ck, points := range pointsByCtx {
		cs.sketches = append(cs.sketches, cs.newSketchSeries(ck, points))
	}
}

func (cs *CheckSampler) commit(timestamp float64) {
	cs.commitSeries(timestamp)
	cs.commitSketches(timestamp)
	cs.contextResolver.expireContexts(timestamp - defaultExpiry)
}

func (cs *CheckSampler) flush() (metrics.Series, metrics.SketchSeriesList) {
	// series
	series := cs.series
	cs.series = make([]*metrics.Serie, 0)

	// sketches
	sketches := cs.sketches
	cs.sketches = make([]metrics.SketchSeries, 0)

	// garbage collect unused bucket deltas
	now := time.Now()
	for ctxKey, lastSeenBucket := range cs.lastSeenBucket {
		if now.Sub(lastSeenBucket) > cs.bucketExpiry {
			delete(cs.lastSeenBucket, ctxKey)
			delete(cs.lastBucketValue, ctxKey)
		}
	}

	return series, sketches
}
