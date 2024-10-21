// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"math"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/tags"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
func newCheckSampler(expirationCount int, expireMetrics bool, contextResolverMetrics bool, statefulTimeout time.Duration, cache *tags.Store, id checkid.ID, tagger tagger.Component) *CheckSampler {
	return &CheckSampler{
		id:                     id,
		series:                 make([]*metrics.Serie, 0),
		sketches:               make(metrics.SketchSeriesList, 0),
		contextResolver:        newCountBasedContextResolver(expirationCount, cache, tagger, string(id)),
		metrics:                metrics.NewCheckMetrics(expireMetrics, statefulTimeout),
		sketchMap:              make(sketchMap),
		lastBucketValue:        make(map[ckey.ContextKey]int64),
		contextResolverMetrics: contextResolverMetrics,
	}
}

func (cs *CheckSampler) addSample(metricSample *metrics.MetricSample) {
	contextKey := cs.contextResolver.trackContext(metricSample)

	if metricSample.Mtype == metrics.DistributionType {
		cs.sketchMap.insert(int64(metricSample.Timestamp), contextKey, metricSample.Value, metricSample.SampleRate)
		return
	}

	if err := cs.metrics.AddSample(contextKey, metricSample, metricSample.Timestamp, 1, pkgconfigsetup.Datadog()); err != nil {
		log.Debugf("Ignoring sample '%s' on host '%s' and tags '%s': %s", metricSample.Name, metricSample.Host, metricSample.Tags, err)
	}
}

func (cs *CheckSampler) newSketchSeries(ck ckey.ContextKey, points []metrics.SketchPoint) *metrics.SketchSeries {
	ctx, _ := cs.contextResolver.get(ck)
	ss := &metrics.SketchSeries{
		Name: ctx.Name,
		Tags: ctx.Tags(),
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

	contextKey := cs.contextResolver.trackContext(bucket)

	// if the bucket is monotonic and we have already seen the bucket we only send the delta
	if bucket.Monotonic {
		lastBucketValue, bucketFound := cs.lastBucketValue[contextKey]
		rawValue := bucket.Value

		cs.lastBucketValue[contextKey] = rawValue

		// Return early so we don't report the first raw value instead of the delta which will cause spikes
		if !bucketFound && !bucket.FlushFirstValue {
			return
		}

		bucket.Value = rawValue - lastBucketValue
	}

	if bucket.Value < 0 {
		log.Warnf("Negative bucket delta %d for metric %s discarding", bucket.Value, bucket.Name)
		return
	}
	if bucket.Value == 0 {
		// noop
		return
	}

	// "if the quantile falls into the highest bucket, the upper bound of the 2nd highest bucket is returned"
	if math.IsInf(bucket.UpperBound, 1) {
		cs.sketchMap.insertInterp(int64(bucket.Timestamp), contextKey, bucket.LowerBound, bucket.LowerBound, uint(bucket.Value))
		return
	}

	log.Tracef(
		"Interpolating %d values over the [%f-%f] bucket",
		bucket.Value, bucket.LowerBound, bucket.UpperBound,
	)
	cs.sketchMap.insertInterp(int64(bucket.Timestamp), contextKey, bucket.LowerBound, bucket.UpperBound, uint(bucket.Value))
}

func (cs *CheckSampler) commitSeries(timestamp float64) {
	series, errors := cs.metrics.Flush(timestamp)
	for ckey, err := range errors {
		context, ok := cs.contextResolver.get(ckey)
		if !ok {
			log.Errorf("Can't resolve context of error '%s': inconsistent context resolver state: context with key '%v' is not tracked", err, ckey)
		} else {
			log.Infof("No value returned for check metric '%s' on host '%s' and tags '%s': %s", context.Name, context.Host, context.Tags().Join(", "), err)
		}
	}
	for _, serie := range series {
		// Resolve context and populate new []Serie
		context, ok := cs.contextResolver.get(serie.ContextKey)
		if !ok {
			log.Errorf("Ignoring all metrics on context key '%v': inconsistent context resolver state: the context is not tracked", serie.ContextKey)
			continue
		}
		serie.Name = context.Name + serie.NameSuffix
		serie.Tags = context.Tags()
		serie.Host = context.Host
		serie.NoIndex = context.noIndex
		serie.SourceTypeName = checksSourceTypeName // this source type is required for metrics coming from the checks
		serie.Source = context.source

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

	cs.metrics.RemoveExpired(timestamp)

	expiredContextKeys := cs.contextResolver.expireContexts()

	// garbage collect unused buckets
	for _, ctxKey := range expiredContextKeys {
		delete(cs.lastBucketValue, ctxKey)
	}

	cs.metrics.Expire(expiredContextKeys, timestamp)
}

func (cs *CheckSampler) flush() (metrics.Series, metrics.SketchSeriesList) {
	// series
	series := cs.series
	cs.series = make([]*metrics.Serie, 0)

	// sketches
	sketches := cs.sketches
	cs.sketches = make(metrics.SketchSeriesList, 0)

	// update sampler metrics
	cs.updateMetrics()

	return series, sketches
}

func (cs *CheckSampler) release() {
	cs.releaseMetrics()
	cs.contextResolver.release()
}

func (cs *CheckSampler) releaseMetrics() {
	if !cs.contextResolverMetrics {
		return
	}
	idString := string(cs.id)
	tlmChecksContexts.Delete(idString)
	for i := 0; i < int(metrics.NumMetricTypes); i++ {
		mtype := metrics.MetricType(i).String()
		tlmChecksContextsByMtype.Delete(idString, mtype)
		tlmChecksContextsBytesByMtype.Delete(idString, mtype)
	}
}

func (cs *CheckSampler) updateMetrics() {
	if !cs.contextResolverMetrics {
		return
	}
	totalContexts := cs.contextResolver.length()
	idString := string(cs.id)

	tlmChecksContexts.Set(float64(totalContexts), idString)
	cs.contextResolver.updateMetrics(tlmChecksContextsByMtype, tlmChecksContextsBytesByMtype)
}
