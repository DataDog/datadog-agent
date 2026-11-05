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
	"github.com/DataDog/datadog-agent/pkg/metrics/metricname"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/quantile"
)

const checksSourceTypeName = "System"

type bucketBounds struct {
	lower, upper float64
}

// CheckSampler aggregates metrics from one Check instance
type CheckSampler struct {
	id                     checkid.ID
	series                 []*metrics.Serie
	sketches               metrics.SketchSeriesList
	mergedSeries           []*metrics.Serie
	mergedSketches         []mergedSketch
	contextResolver        *countBasedContextResolver
	metrics                metrics.CheckMetrics
	sketchMap              sketchMap
	lastBucketValue        map[ckey.ContextKey]int64
	lastBucketValueByBound map[ckey.ContextKey]map[bucketBounds]int64
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
	contextResolver := newCountBasedContextResolver(expirationCount, cache, tagger, string(id))
	contextResolver.resolver.stripAtOutput = true
	return &CheckSampler{
		id:                     id,
		series:                 make([]*metrics.Serie, 0),
		sketches:               make(metrics.SketchSeriesList, 0),
		contextResolver:        contextResolver,
		metrics:                metrics.NewCheckMetrics(expireMetrics, statefulTimeout),
		sketchMap:              make(sketchMap),
		lastBucketValue:        make(map[ckey.ContextKey]int64),
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
		// The observer sees the identity the metric is sent with, stripped by metric_tag_filterlist rules.
		context, _ := cs.contextResolver.get(contextKey)
		view, observedKey := resolvedMetricView{sample: metricSample, host: context.Host, tags: context.Tags()}, contextKey
		if stripped, ok := cs.contextResolver.resolver.strippedOf(context); ok {
			view.tags, observedKey = stripped.Tags(), stripped.key
		}
		cs.observerHandle.ObserveMetric(view, uint64(observedKey))
	}
	if metricSample.Mtype == metrics.DistributionType {
		cs.sketchMap.insert(int64(metricSample.Timestamp), contextKey, metricSample.Value, metricSample.SampleRate)
		return
	}

	if err := cs.metrics.AddSample(contextKey, metricSample, metricSample.Timestamp, 1, pkgconfigsetup.Datadog()); err != nil {
		log.Debugf("Ignoring sample '%s' on host '%s' and tags '%s': %s", metricSample.Name, metricSample.Host, metricSample.Tags, err)
	}
}

func (cs *CheckSampler) newSketchSeries(ctx *Context, points []metrics.SketchPoint) *metrics.SketchSeries {
	ss := &metrics.SketchSeries{
		DistributionMetadata: metrics.DistributionMetadata{
			Name:   ctx.Name,
			Tags:   ctx.Tags(),
			Host:   ctx.Host,
			Source: ctx.source,
			// Interval: TODO: investigate
		},
		Points: points,
	}

	return ss
}

// mergedIdentity returns the key and tags of the series a context's output is merged into: stripped while a rule
// applies, and its own otherwise.
func (cs *CheckSampler) mergedIdentity(ck ckey.ContextKey, ctx *Context) (ckey.ContextKey, tagset.CompositeTags) {
	if stripped, ok := cs.contextResolver.resolver.strippedOf(ctx); ok {
		return stripped.key, stripped.Tags()
	}
	return ck, ctx.Tags()
}

// releasesAtCommit returns whether a context's sketches are all released at every commit, stamped at the commit,
// instead of per second of submission.
func (cs *CheckSampler) releasesAtCommit(ck ckey.ContextKey) bool {
	ctx, ok := cs.contextResolver.get(ck)
	return ok && ctx.mergesAtOutput()
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

	// "if the quantile falls into the highest bucket, the upper bound of the 2nd highest bucket is returned"
	if math.IsInf(bucket.UpperBound, 1) {
		cs.sketchMap.insertInterp(int64(bucket.Timestamp), contextKey, bucket.LowerBound, bucket.LowerBound, uint(bucket.Value))
		return
	}

	// mirror the +Inf case: the open-ended bottom bucket collapses to its upper bound
	if math.IsInf(bucket.LowerBound, -1) {
		cs.sketchMap.insertInterp(int64(bucket.Timestamp), contextKey, bucket.UpperBound, bucket.UpperBound, uint(bucket.Value))
		return
	}

	log.Tracef(
		"Interpolating %d values over the [%f-%f] bucket",
		bucket.Value, bucket.LowerBound, bucket.UpperBound,
	)
	cs.sketchMap.insertInterp(int64(bucket.Timestamp), contextKey, bucket.LowerBound, bucket.UpperBound, uint(bucket.Value))
}

func (cs *CheckSampler) commitSeries(timestamp float64, filterList *metricname.Matcher) {

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
		// Context keys don't include the metric type, so a context can also get samples of a type whose series
		// can't be merged, such as an ordinary histogram's statistics: only series that add up are merged.
		if context.mergesSeries() && mergeableSerie(serie) {
			serie.ContextKey, serie.Tags = cs.mergedIdentity(serie.ContextKey, context)
			cs.mergedSeries = append(cs.mergedSeries, serie)
			continue
		}

		cs.series = append(cs.series, serie)
	}
}

// commitSketches releases the sketches from before the commit second. The sketches of contexts merged at output are
// all released, merged into one point stamped at the commit, so that they can be merged with other sources' commits
// in the same second.
func (cs *CheckSampler) commitSketches(timestamp float64, filterList *metricname.Matcher) {
	commitTs := int64(timestamp)
	pointsByCtx := make(map[ckey.ContextKey][]metrics.SketchPoint)
	perSecond := func(ck ckey.ContextKey, p metrics.SketchPoint) {
		if p.Sketch != nil {
			pointsByCtx[ck] = append(pointsByCtx[ck], p)
		}
	}

	merges := cs.contextResolver.resolver.anyStrippedMetric()
	if !merges {
		cs.sketchMap.flushBefore(commitTs, perSecond)
	} else {
		var atCommit map[ckey.ContextKey]*quantile.Sketch
		stampAtCommit := func(ck ckey.ContextKey, p metrics.SketchPoint) {
			if p.Sketch == nil {
				return
			}
			if atCommit == nil {
				atCommit = make(map[ckey.ContextKey]*quantile.Sketch)
			}
			if sketch, ok := atCommit[ck]; ok {
				sketch.Merge(quantile.Default(), p.Sketch)
			} else {
				atCommit[ck] = p.Sketch
			}
		}
		cs.sketchMap.flushIf(func(ts int64, ck ckey.ContextKey) func(ckey.ContextKey, metrics.SketchPoint) {
			if cs.releasesAtCommit(ck) {
				return stampAtCommit
			}
			if ts < commitTs {
				return perSecond
			}
			return nil
		})
		for ck, sketch := range atCommit {
			pointsByCtx[ck] = []metrics.SketchPoint{{Sketch: sketch, Ts: commitTs}}
		}
	}
	for ck, points := range pointsByCtx {
		ctx, ok := cs.contextResolver.get(ck)
		if !ok {
			log.Errorf("Ignoring sketch on context key '%v': inconsistent context resolver state: the context is not tracked", ck)
			continue
		}
		series := cs.newSketchSeries(ctx, points)
		// Filter the metrics
		if filterList != nil && filterList.Test(series.Name) {
			tlmChecksFilteredMetrics.Inc()
			continue
		}
		if merges && ctx.mergesAtOutput() {
			var key ckey.ContextKey
			key, series.Tags = cs.mergedIdentity(ck, ctx)
			cs.mergedSketches = append(cs.mergedSketches, mergedSketch{key: key, sketch: series})
			continue
		}
		cs.sketches = append(cs.sketches, series)
	}
}

func (cs *CheckSampler) commit(timestamp float64, filterList *metricname.Matcher) {
	cs.commitSeries(timestamp, filterList)
	cs.commitSketches(timestamp, filterList)

	cs.metrics.RemoveExpired(timestamp)

	expiredContextKeys := cs.contextResolver.expireContexts()

	// garbage collect unused buckets
	for _, ctxKey := range expiredContextKeys {
		delete(cs.lastBucketValue, ctxKey)
		delete(cs.lastBucketValueByBound, ctxKey)
	}

	cs.metrics.Expire(expiredContextKeys, timestamp)
}

// flushMerged returns the output committed since the last call that goes to the check output merger.
func (cs *CheckSampler) flushMerged() ([]*metrics.Serie, []mergedSketch) {
	series, sketches := cs.mergedSeries, cs.mergedSketches
	cs.mergedSeries, cs.mergedSketches = nil, nil
	return series, sketches
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
		tlmChecksContextsBytesByMtype.Delete(idString, mtype, tags.BytesKindStruct)
		tlmChecksContextsBytesByMtype.Delete(idString, mtype, tags.BytesKindData)
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

// restrip updates the identity every context strips to at output, after the rules changed. Once every sampler
// restripped, markStrippedMetrics must be called on each.
func (cs *CheckSampler) restrip(filterList filterlist.TagMatcher) {
	cs.contextResolver.resolver.restrip(filterList)
}

// markStrippedMetrics marks the contexts of metrics stripped since the Agent started, after the rules changed.
func (cs *CheckSampler) markStrippedMetrics() {
	cs.contextResolver.resolver.markStrippedMetrics()
}

// shareStrippedIdentities makes the sampler record stripped identities in a table shared with other samplers.
// It must be called before the sampler tracks any context.
func (cs *CheckSampler) shareStrippedIdentities(table *strippedIdentityTable) {
	cs.contextResolver.resolver.stripped = table
}

// mergeableSerie returns whether a series of a check metric adds up across sources: the single series of a counter,
// count or monotonic count, not the statistics of a histogram, nor a gauge or rate.
func mergeableSerie(serie *metrics.Serie) bool {
	return serie.NameSuffix == "" && (serie.MType == metrics.APICountType || serie.MType == metrics.APIRateType)
}
