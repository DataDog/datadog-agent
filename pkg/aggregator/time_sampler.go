// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"fmt"
	"io"
	"math"
	"strconv"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	filterlist "github.com/DataDog/datadog-agent/comp/filterlist/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/tags"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/quantile"
	utilstrings "github.com/DataDog/datadog-agent/pkg/util/strings"
)

// SerieSignature holds the elements that allow to know whether two similar `Serie`s
// from the same bucket can be merged into one. Series must have the same contextKey.
type SerieSignature struct {
	mType      metrics.APIMetricType
	nameSuffix string
}

// TimeSamplerID is a type ID for sharded time samplers.
type TimeSamplerID int

// TimeSampler aggregates metrics by buckets of 'interval' seconds
type TimeSampler struct {
	interval           int64
	contextResolver    *timestampContextResolver
	metricsByTimestamp map[int64]metrics.ContextMetrics
	lastCutOffTime     int64
	sketchMap          sketchMap

	// id is a number to differentiate multiple time samplers
	// since we start running more than one with the demultiplexer introduction
	id       TimeSamplerID
	idString string

	hostname string
}

// NewTimeSampler returns a newly initialized TimeSampler
func NewTimeSampler(id TimeSamplerID, interval int64, cache *tags.Store, tagger tagger.Component, hostname string) *TimeSampler {
	if interval == 0 {
		interval = bucketSize
	}

	idString := strconv.Itoa(int(id))
	log.Infof("Creating TimeSampler #%s", idString)

	contextExpireTime := pkgconfigsetup.Datadog().GetInt64("dogstatsd_context_expiry_seconds")
	counterExpireTime := contextExpireTime + pkgconfigsetup.Datadog().GetInt64("dogstatsd_expiry_seconds")

	s := &TimeSampler{
		interval:           interval,
		contextResolver:    newTimestampContextResolver(tagger, cache, idString, contextExpireTime, counterExpireTime),
		metricsByTimestamp: map[int64]metrics.ContextMetrics{},
		sketchMap:          make(sketchMap),
		id:                 id,
		idString:           idString,
		hostname:           hostname,
	}

	return s
}

func (s *TimeSampler) calculateBucketStart(timestamp float64) int64 {
	return int64(timestamp) - int64(timestamp)%s.interval
}

func (s *TimeSampler) isBucketStillOpen(bucketStartTimestamp, timestamp int64) bool {
	return bucketStartTimestamp+s.interval > timestamp
}

func (s *TimeSampler) sample(metricSample *metrics.MetricSample, timestamp float64) {
	// use the timestamp provided in the sample if any
	if metricSample.Timestamp > 0 {
		timestamp = metricSample.Timestamp
	}

	// Keep track of the context
	contextKey := s.contextResolver.trackContext(metricSample, int64(timestamp))
	bucketStart := s.calculateBucketStart(timestamp)

	switch metricSample.Mtype {
	case metrics.DistributionType:
		s.sketchMap.insert(bucketStart, contextKey, metricSample.Value, metricSample.SampleRate)
	default:
		// If it's a new bucket, initialize it
		bucketMetrics, ok := s.metricsByTimestamp[bucketStart]
		if !ok {
			bucketMetrics = metrics.MakeContextMetrics()
			s.metricsByTimestamp[bucketStart] = bucketMetrics
		}
		// Add sample to bucket
		if err := bucketMetrics.AddSample(contextKey, metricSample, timestamp, s.interval, nil, pkgconfigsetup.Datadog()); err != nil {
			log.Debugf("TimeSampler #%d Ignoring sample '%s' on host '%s' and tags '%s': %s", s.id, metricSample.Name, metricSample.Host, metricSample.Tags, err)
		}
	}
}

func (s *TimeSampler) flushSeries(cutoffTime int64, series metrics.SerieSink, filterList *utilstrings.Matcher, forceFlushAll bool) {
	// Map to hold the expired contexts that will need to be deleted after the flush so that we stop sending zeros
	contextMetricsFlusher := metrics.NewContextMetricsFlusher()

	if len(s.metricsByTimestamp) > 0 {
		for bucketTimestamp, contextMetrics := range s.metricsByTimestamp {
			// disregard when the timestamp is too recent
			if s.isBucketStillOpen(bucketTimestamp, cutoffTime) && !forceFlushAll {
				continue
			}

			// Add a 0 sample to all the counters that are not expired.
			// It is ok to add 0 samples to a counter that was already sampled for real in the bucket, since it won't change its value
			s.countersSampleZeroValue(bucketTimestamp, contextMetrics)
			contextMetricsFlusher.Append(float64(bucketTimestamp), contextMetrics)

			delete(s.metricsByTimestamp, bucketTimestamp)
		}
	} else if s.lastCutOffTime+s.interval <= cutoffTime {
		// Even if there is no metric in this flush, recreate empty counters,
		// but only if we've passed an interval since the last flush

		contextMetrics := metrics.MakeContextMetrics()

		s.countersSampleZeroValue(cutoffTime-s.interval, contextMetrics)
		contextMetricsFlusher.Append(float64(cutoffTime-s.interval), contextMetrics)
	}

	// serieBySignature is reused for each call of dedupSerieBySerieSignature to avoid allocations.
	serieBySignature := make(map[SerieSignature]*metrics.Serie)
	s.flushContextMetrics(contextMetricsFlusher, func(rawSeries []*metrics.Serie) {
		// Note: rawSeries is reused at each call
		s.dedupSerieBySerieSignature(rawSeries, series, serieBySignature, filterList)
	})
}

func (s *TimeSampler) dedupSerieBySerieSignature(
	rawSeries []*metrics.Serie,
	serieSink metrics.SerieSink,
	serieBySignature map[SerieSignature]*metrics.Serie,
	filterList *utilstrings.Matcher,
) {
	// clear the map. Reuse serieBySignature
	for k := range serieBySignature {
		delete(serieBySignature, k)
	}

	// rawSeries have the same context key.
	for _, serie := range rawSeries {
		serieSignature := SerieSignature{serie.MType, serie.NameSuffix}

		if existingSerie, ok := serieBySignature[serieSignature]; ok {
			existingSerie.Points = append(existingSerie.Points, serie.Points[0])
		} else {
			// Resolve context and populate new Serie
			context, ok := s.contextResolver.get(serie.ContextKey)
			if !ok {
				log.Errorf("TimeSampler #%d Ignoring all metrics on context key '%v': inconsistent context resolver state: the context is not tracked", s.id, serie.ContextKey)
				continue
			}
			serie.Name = context.Name + serie.NameSuffix
			serie.Tags = context.Tags()
			serie.Host = context.Host
			serie.NoIndex = context.noIndex
			serie.Interval = s.interval
			serie.Source = context.source

			serieBySignature[serieSignature] = serie
		}
	}

	for _, serie := range serieBySignature {
		// it is the final stage before flushing the series to the serialisation
		// part of the pipeline but also, here is a stage where all series have been
		// generated & processed (even the ones generated from a histogram metric).
		if filterList != nil && filterList.Test(serie.Name) {
			tlmDogstatsdFilteredMetrics.Inc()
			continue
		}
		serieSink.Append(serie)
	}
}

func (s *TimeSampler) flushSketches(cutoffTime int64, sketchesSink metrics.SketchesSink, forceFlushAll bool, tagFilter filterlist.TagMatcher) {
	pointsByCtx := make(map[ckey.ContextKey][]metrics.SketchPoint)

	flushAllBefore := cutoffTime
	if forceFlushAll {
		flushAllBefore = math.MaxInt64
	}

	s.sketchMap.flushBefore(flushAllBefore, func(ck ckey.ContextKey, p metrics.SketchPoint) {
		if p.Sketch == nil {
			return
		}
		pointsByCtx[ck] = append(pointsByCtx[ck], p)
	})

	// pointsByStrippedCtx: stripped context key -> (timestamp -> merged sketch)
	pointsByStrippedCtx := make(map[ckey.ContextKey]map[int64]*quantile.Sketch, len(pointsByCtx))
	// firstCtxByStrippedKey: stripped key -> first original context seen (for metadata)
	firstCtxByStrippedKey := make(map[ckey.ContextKey]*Context, len(pointsByCtx))
	// tagsForStrippedKey: stripped key -> composite tags after filtering
	tagsForStrippedKey := make(map[ckey.ContextKey]tagset.CompositeTags, len(pointsByCtx))

	for ck, points := range pointsByCtx {
		ctx, ok := s.contextResolver.get(ck)
		if !ok {
			log.Errorf("TimeSampler #%d Ignoring all metrics on context key '%v': inconsistent context resolver state: the context is not tracked", s.id, ck)
			continue
		}

		strippedKey := ck
		ctxTags := ctx.Tags()
		if keepTag, strip := tagFilter.ShouldStripTags(ctx.Name); strip {
			strippedKey, ctxTags = s.computeStrippedKey(ctx, keepTag)
		}

		if _, exists := firstCtxByStrippedKey[strippedKey]; !exists {
			firstCtxByStrippedKey[strippedKey] = ctx
			tagsForStrippedKey[strippedKey] = ctxTags
			pointsByStrippedCtx[strippedKey] = make(map[int64]*quantile.Sketch, len(points))
		}

		for _, p := range points {
			if existing := pointsByStrippedCtx[strippedKey][p.Ts]; existing != nil {
				existing.Merge(quantile.Default(), p.Sketch)
			} else {
				pointsByStrippedCtx[strippedKey][p.Ts] = p.Sketch
			}
		}
	}

	for strippedCk, pointsByTs := range pointsByStrippedCtx {
		ctx := firstCtxByStrippedKey[strippedCk]
		points := make([]metrics.SketchPoint, 0, len(pointsByTs))
		for ts, sketch := range pointsByTs {
			points = append(points, metrics.SketchPoint{Sketch: sketch, Ts: ts})
		}
		ss := &metrics.SketchSeries{
			Name:       ctx.Name,
			Tags:       tagsForStrippedKey[strippedCk],
			Host:       ctx.Host,
			Interval:   s.interval,
			Points:     points,
			ContextKey: strippedCk,
			Source:     ctx.source,
			NoIndex:    ctx.noIndex,
		}
		sketchesSink.Append(ss)
	}
}

// computeStrippedKey generates a context key and filtered tags for the given context,
// applying the keepTag filter to both tagger and metric tags.
// Reuses the contextResolver's shared buffers and key generator (same pattern as trackContext).
func (s *TimeSampler) computeStrippedKey(ctx *Context, keepTag func(string) bool) (ckey.ContextKey, tagset.CompositeTags) {
	cr := s.contextResolver.resolver
	cr.taggerBuffer.IncludeAll = false
	cr.taggerBuffer.IncludeTag = keepTag
	cr.metricBuffer.IncludeAll = false
	cr.metricBuffer.IncludeTag = keepTag
	cr.taggerBuffer.Append(ctx.taggerTags.Tags()...)
	cr.metricBuffer.Append(ctx.metricTags.Tags()...)
	key, _, _ := cr.keyGenerator.GenerateWithTags2(ctx.Name, ctx.Host, cr.taggerBuffer, cr.metricBuffer)
	// Capture tags after GenerateWithTags2 deduplication
	filteredTags := tagset.NewCompositeTags(
		append([]string(nil), cr.taggerBuffer.Get()...),
		append([]string(nil), cr.metricBuffer.Get()...),
	)
	cr.taggerBuffer.Reset()
	cr.metricBuffer.Reset()
	return key, filteredTags
}

func (s *TimeSampler) flush(timestamp float64, series metrics.SerieSink, sketches metrics.SketchesSink, filterList *utilstrings.Matcher, forceFlushAll bool, tagFilter filterlist.TagMatcher) {
	// Compute a limit timestamp
	cutoffTime := s.calculateBucketStart(timestamp)

	s.flushSeries(cutoffTime, series, filterList, forceFlushAll)
	s.flushSketches(cutoffTime, sketches, forceFlushAll, tagFilter)
	// expiring contexts
	s.contextResolver.expireContexts(int64(timestamp))
	s.lastCutOffTime = cutoffTime

	s.updateMetrics()
	s.sendTelemetry(timestamp, series)
}

// We do this here mostly because we want to avoid slow operations when we track/remove
// contexts in the contextResolver. Keeping references to the metrics in the contextResolver
// would probably be enough to avoid this.
func (s *TimeSampler) updateMetrics() {
	totalContexts := s.contextResolver.length()
	aggregatorDogstatsdContexts.Set(int64(totalContexts))
	tlmDogstatsdContexts.Set(float64(totalContexts), s.idString)
	tlmDogstatsdTimeBuckets.Set(float64(len(s.metricsByTimestamp)), s.idString)

	countByMtype := s.contextResolver.countsByMtype()
	for i := 0; i < int(metrics.NumMetricTypes); i++ {
		count := countByMtype[i]

		aggregatorDogstatsdContextsByMtype[i].Set(int64(count))
	}
	s.contextResolver.updateMetrics(tlmDogstatsdContextsByMtype, tlmDogstatsdContextsBytesByMtype)
}

// flushContextMetrics flushes the contextMetrics inside contextMetricsFlusher, handles its errors,
// and call several times `callback`, each time with series with same context key
func (s *TimeSampler) flushContextMetrics(contextMetricsFlusher *metrics.ContextMetricsFlusher, callback func([]*metrics.Serie)) {
	errors := contextMetricsFlusher.FlushAndClear(callback)
	for ckey, err := range errors {
		context, ok := s.contextResolver.get(ckey)
		if !ok {
			log.Errorf("Can't resolve context of error '%s': inconsistent context resolver state: context with key '%v' is not tracked", err, ckey)
			continue
		}
		log.Infof("No value returned for dogstatsd metric '%s' on host '%s' and tags '%s': %s", context.Name, context.Host, context.Tags(), err)
	}
}

func (s *TimeSampler) countersSampleZeroValue(timestamp int64, contextMetrics metrics.ContextMetrics) {
	expirySeconds := pkgconfigsetup.Datadog().GetInt64("dogstatsd_expiry_seconds")
	for counterContext, entry := range s.contextResolver.resolver.contextsByKey {
		if entry.lastSeen+expirySeconds > timestamp && entry.context.mtype == metrics.CounterType {
			sample := &metrics.MetricSample{
				Name:       "",
				Value:      0.0,
				RawValue:   "0.0",
				Mtype:      metrics.CounterType,
				Tags:       []string{},
				Host:       "",
				SampleRate: 1,
				Timestamp:  float64(timestamp),
			}
			// Add a zero value sample to the counter
			// It is ok to add a 0 sample to a counter that was already sampled in the bucket, it won't change its value
			contextMetrics.AddSample(counterContext, sample, float64(timestamp), s.interval, nil, pkgconfigsetup.Datadog()) //nolint:errcheck
		}
	}
}

func (s *TimeSampler) sendTelemetry(timestamp float64, series metrics.SerieSink) {
	if !pkgconfigsetup.Datadog().GetBool("telemetry.enabled") {
		return
	}

	// If multiple samplers are used, this avoids the need to
	// aggregate the stats agent-side, and allows us to see amount of
	// tags duplication between shards.
	tags := []string{
		fmt.Sprintf("sampler_id:%d", s.id),
	}

	if pkgconfigsetup.Datadog().GetBool("telemetry.dogstatsd_origin") {
		s.contextResolver.sendOriginTelemetry(timestamp, series, s.hostname, tags)
	}
}

func (s *TimeSampler) dumpContexts(dest io.Writer) error {
	return s.contextResolver.dumpContexts(dest)
}
