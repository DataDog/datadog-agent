// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"math"
	"time"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	filterlist "github.com/DataDog/datadog-agent/comp/filterlist/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/tags"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/util"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	utilstrings "github.com/DataDog/datadog-agent/pkg/util/strings"
)

const checksSourceTypeName = "System"

type bucketBounds struct {
	lower, upper float64
}

type sketchSummaryObserverSample struct {
	name      string
	value     float64
	tags      []string
	timestamp int64
	source    metrics.MetricSource
}

func (s sketchSummaryObserverSample) GetName() string {
	return s.name
}

func (s sketchSummaryObserverSample) GetValue() float64 {
	return s.value
}

func (s sketchSummaryObserverSample) GetRawTags() []string {
	return s.tags
}

func (s sketchSummaryObserverSample) GetTimestampUnix() int64 {
	return s.timestamp
}

func (s sketchSummaryObserverSample) GetSampleRate() float64 {
	return 1
}

func (s sketchSummaryObserverSample) GetSource() metrics.MetricSource {
	return s.source
}

// CheckSampler aggregates metrics from one Check instance
type CheckSampler struct {
	id                     checkid.ID
	series                 []*metrics.Serie
	sketches               metrics.SketchSeriesList
	contextResolver        *countBasedContextResolver
	metrics                metrics.CheckMetrics
	sketchMap              sketchMap
	lastBucketValue        map[ckey.ContextKey]int64
	lastBucketValueByBound map[ckey.ContextKey]map[bucketBounds]int64
	histogramSketchContext map[ckey.ContextKey]struct{}
	deregistered           bool
	contextResolverMetrics bool
	logThrottling          util.SimpleThrottler
	allowSketchBucketReset bool
	observerHandle         observer.Handle
}

// newCheckSampler returns a newly initialized CheckSampler
func newCheckSampler(
	expirationCount int,
	expireMetrics bool,
	contextResolverMetrics bool,
	statefulTimeout time.Duration,
	allowSketchBucketReset bool,
	cache *tags.Store,
	id checkid.ID,
	tagger tagger.Component,
) *CheckSampler {
	return &CheckSampler{
		id:                     id,
		series:                 make([]*metrics.Serie, 0),
		sketches:               make(metrics.SketchSeriesList, 0),
		contextResolver:        newCountBasedContextResolver(expirationCount, cache, tagger, string(id)),
		metrics:                metrics.NewCheckMetrics(expireMetrics, statefulTimeout),
		sketchMap:              make(sketchMap),
		lastBucketValue:        make(map[ckey.ContextKey]int64),
		// histogramSketchContext is lazily allocated in addBucket only
		// when an observer handle is attached. With a nil handle (the
		// default and the production state until anomaly_detection is
		// enabled), this map stays nil and the bucket path pays nothing
		// for the histogram-summary observer feature.
		contextResolverMetrics: contextResolverMetrics,
		logThrottling:          util.NewSimpleThrottler(5, 5*time.Minute, ""),
		allowSketchBucketReset: allowSketchBucketReset,
	}
}

// SetObserverHandle sets the observer handle for mirroring check samples.
func (cs *CheckSampler) SetObserverHandle(h observer.Handle) {
	cs.observerHandle = h
}

func (cs *CheckSampler) addSample(metricSample *metrics.MetricSample, tagFilterList filterlist.TagMatcher) {
	contextKey := cs.contextResolver.trackContext(metricSample, tagFilterList)
	if cs.observerHandle != nil {
		cs.observerHandle.ObserveMetric(metricSample)
	}
	if metricSample.Mtype == metrics.DistributionType {
		cs.sketchMap.insert(int64(metricSample.Timestamp), contextKey, metricSample.Value, metricSample.SampleRate)
		return
	}

	if err := cs.metrics.AddSample(contextKey, metricSample, metricSample.Timestamp, 1, pkgconfigsetup.Datadog()); err != nil {
		log.Debugf("Ignoring sample '%s' on host '%s' and tags '%s': %s", metricSample.Name, metricSample.Host, metricSample.Tags, err)
	}
}

func (cs *CheckSampler) newSketchSeries(ck ckey.ContextKey, points []metrics.SketchPoint) *metrics.SketchSeries {
	ctx, ok := cs.contextResolver.get(ck)
	if !ok {
		log.Errorf("Ignoring sketch on context key '%v': inconsistent context resolver state: the context is not tracked", ck)
		return nil
	}
	ss := &metrics.SketchSeries{
		Name: ctx.Name,
		Tags: ctx.Tags(),
		Host: ctx.Host,
		// Interval: TODO: investigate
		Points:     points,
		ContextKey: ck,
		Source:     ctx.source,
	}

	return ss
}

func (cs *CheckSampler) addBucket(bucket *metrics.HistogramBucket, tagFilterList filterlist.TagMatcher) {
	if bucket.Value < 0 {
		if !cs.logThrottling.ShouldThrottle() {
			log.Warnf("Negative bucket value %d for metric %s discarding", bucket.Value, bucket.Name)
		}
		return
	}
	if bucket.Value == 0 {
		// noop
		return
	}

	bucketRange := bucket.UpperBound - bucket.LowerBound
	if bucketRange < 0 {
		if !cs.logThrottling.ShouldThrottle() {
			log.Warnf(
				"Negative bucket range [%f-%f] for metric %s discarding",
				bucket.LowerBound, bucket.UpperBound, bucket.Name,
			)
		}
		return
	}

	contextKey := cs.contextResolver.trackContext(bucket, tagFilterList)

	// if the bucket is monotonic and we have already seen the bucket we only send the delta
	if bucket.Monotonic {
		lastBucketValue := int64(0)
		bucketFound := false
		rawValue := bucket.Value

		// Openmetrics checks can send lots of metrics with lots of
		// buckets, but include bucket bounds in the tags and only have
		// one bucket per context key. It pays to use a simpler map to
		// track bucket values for such metrics.
		if !bucket.MultipleBuckets {
			lastBucketValue, bucketFound = cs.lastBucketValue[contextKey]
			cs.lastBucketValue[contextKey] = rawValue
		} else {
			if cs.lastBucketValueByBound == nil {
				cs.lastBucketValueByBound = make(map[ckey.ContextKey]map[bucketBounds]int64)
			}
			lastBucketValues := cs.lastBucketValueByBound[contextKey]
			if lastBucketValues == nil {
				lastBucketValues = make(map[bucketBounds]int64)
				cs.lastBucketValueByBound[contextKey] = lastBucketValues
			}
			bucketBounds := bucketBoundsFor(bucket)
			lastBucketValue, bucketFound = lastBucketValues[bucketBounds]
			lastBucketValues[bucketBounds] = rawValue
		}

		// Return early so we don't report the first raw value instead of the delta which will cause spikes
		if !bucketFound && !bucket.FlushFirstValue {
			return
		}

		// Handle reset for monotonic buckets.
		if rawValue < lastBucketValue && cs.allowSketchBucketReset {
			if !bucket.FlushFirstValue {
				return
			}
		} else {
			bucket.Value = rawValue - lastBucketValue
		}
	}

	if bucket.Value < 0 {
		if !cs.logThrottling.ShouldThrottle() {
			log.Warnf("Negative bucket delta %d for metric %s discarding", bucket.Value, bucket.Name)
		}
		return
	}

	if bucket.Value == 0 {
		// noop
		return
	}

	// Mark this context as histogram-bucket-derived so commitSketches
	// can emit a per-commit observer summary later. Only allocate when an
	// observer handle is actually attached — keeps the no-observer path
	// allocation-free per CheckSamplerCoreBehaviorPreserved.
	if cs.observerHandle != nil {
		if cs.histogramSketchContext == nil {
			cs.histogramSketchContext = make(map[ckey.ContextKey]struct{})
		}
		cs.histogramSketchContext[contextKey] = struct{}{}
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

func (cs *CheckSampler) commitSeries(timestamp float64, filterList *utilstrings.Matcher) {

	series, errors := cs.metrics.Flush(timestamp)
	for ckey, err := range errors {
		context, ok := cs.contextResolver.get(ckey)
		if !ok {
			log.Errorf("Can't resolve context of error '%s': inconsistent context resolver state: context with key '%v' is not tracked", err, ckey)
		} else {
			log.Debugf("No value returned for check metric '%s' on host '%s' and tags '%s': %s", context.Name, context.Host, context.Tags().Join(", "), err)
		}
	}
	for _, serie := range series {
		// Resolve context and populate new []Serie
		context, ok := cs.contextResolver.get(serie.ContextKey)
		if !ok {
			log.Errorf("Ignoring all metrics on context key '%v': inconsistent context resolver state: the context is not tracked", serie.ContextKey)
			continue
		}

		name := context.Name + serie.NameSuffix
		// Filter the metrics
		if filterList != nil && filterList.Test(name) {
			tlmChecksFilteredMetrics.Inc()
			continue
		}
		serie.Name = name
		serie.Tags = context.Tags()
		serie.Host = context.Host
		serie.NoIndex = context.noIndex
		serie.SourceTypeName = checksSourceTypeName // this source type is required for metrics coming from the checks
		serie.Source = context.source

		cs.series = append(cs.series, serie)
	}
}

func (cs *CheckSampler) commitSketches(timestamp float64, filterList *utilstrings.Matcher) {
	pointsByCtx := make(map[ckey.ContextKey][]metrics.SketchPoint)

	cs.sketchMap.flushBefore(int64(timestamp), func(ck ckey.ContextKey, p metrics.SketchPoint) {
		if p.Sketch == nil {
			return
		}
		pointsByCtx[ck] = append(pointsByCtx[ck], p)
	})
	for ck, points := range pointsByCtx {
		series := cs.newSketchSeries(ck, points)
		if series == nil {
			continue
		}
		if _, ok := cs.histogramSketchContext[ck]; ok {
			cs.observeSketchSummary(series)
			delete(cs.histogramSketchContext, ck)
		}
		// Filter the metrics
		if filterList != nil && filterList.Test(series.Name) {
			tlmChecksFilteredMetrics.Inc()
			continue
		}
		cs.sketches = append(cs.sketches, series)
	}
}

func (cs *CheckSampler) observeSketchSummary(series *metrics.SketchSeries) {
	if cs.observerHandle == nil {
		return
	}

	// The observer/ring-buffer stores scalar time series, not sketches.
	// Record fixed-cardinality BasicStats summaries for each committed
	// sketch point so local queries keep 1Hz activity/value context without
	// storing full sketches or one series per histogram bucket.
	for _, point := range series.Points {
		if point.Sketch == nil {
			continue
		}
		count, min, max, sum, avg := point.Sketch.BasicStats()
		if count == 0 {
			continue
		}
		tags := sketchSummaryTags(series)
		cs.observeSketchSummaryMetric(series, point.Ts, tags, "count", float64(count))
		cs.observeSketchSummaryMetric(series, point.Ts, tags, "sum", sum)
		cs.observeSketchSummaryMetric(series, point.Ts, tags, "min", min)
		cs.observeSketchSummaryMetric(series, point.Ts, tags, "max", max)
		cs.observeSketchSummaryMetric(series, point.Ts, tags, "avg", avg)
	}
}

func sketchSummaryTags(series *metrics.SketchSeries) []string {
	tags := make([]string, 0, series.Tags.Len()+2)
	series.Tags.ForEach(func(tag string) {
		tags = append(tags, tag)
	})
	return append(tags, "observer_metric_type:sketch_summary")
}

func (cs *CheckSampler) observeSketchSummaryMetric(series *metrics.SketchSeries, timestamp int64, baseTags []string, statistic string, value float64) {
	tags := make([]string, 0, len(baseTags)+1)
	tags = append(tags, baseTags...)
	tags = append(tags, "sketch_stat:"+statistic)
	cs.observerHandle.ObserveMetric(sketchSummaryObserverSample{
		name:      series.Name + "." + statistic,
		value:     value,
		tags:      tags,
		timestamp: timestamp,
		source:    series.Source,
	})
}

func (cs *CheckSampler) commit(timestamp float64, filterList *utilstrings.Matcher) {
	cs.commitSeries(timestamp, filterList)
	cs.commitSketches(timestamp, filterList)

	cs.metrics.RemoveExpired(timestamp)

	expiredContextKeys := cs.contextResolver.expireContexts()

	// garbage collect unused buckets
	for _, ctxKey := range expiredContextKeys {
		delete(cs.lastBucketValue, ctxKey)
		delete(cs.lastBucketValueByBound, ctxKey)
		delete(cs.histogramSketchContext, ctxKey)
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

func (cs *CheckSampler) clearStripCache() {
	cs.contextResolver.resolver.clearTagFilterCache()
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

func bucketBoundsFor(bucket *metrics.HistogramBucket) bucketBounds {
	return bucketBounds{
		lower: bucket.LowerBound,
		upper: bucket.UpperBound,
	}
}
