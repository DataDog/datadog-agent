// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"fmt"
	"io"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/tags"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// SerieSignature holds the elements that allow to know whether two similar `Serie`s
// from the same bucket can be merged into one. Series must have the same contextKey.
type SerieSignature struct {
	mType      metrics.APIMetricType
	nameSuffix string
}

// TimeSamplerID is a type ID for sharded time samplers.
type TimeSamplerID int

type metricsMap map[int64]metrics.ContextMetrics

// Immutable part of the timeSampler that can be shared with async flush
type timeSamplerConst struct {
	// id is a number to differentiate multiple time samplers
	// since we start running more than one with the demultiplexer introduction
	id       TimeSamplerID
	idString string
	interval int64
	hostname string
}

// TimeSampler aggregates metrics by buckets of 'interval' seconds
type TimeSampler struct {
	timeSamplerConst

	contextResolver    *timestampContextResolver
	metricsByTimestamp map[int64]metrics.ContextMetrics
	lastCutOffTime     int64
	sketchMap          sketchMap
}

// NewTimeSampler returns a newly initialized TimeSampler
func NewTimeSampler(id TimeSamplerID, interval int64, cache *tags.Store, hostname string) *TimeSampler {
	if interval == 0 {
		interval = bucketSize
	}

	idString := strconv.Itoa(int(id))
	log.Infof("Creating TimeSampler #%s", idString)

	contextExpireTime := config.Datadog().GetInt64("dogstatsd_context_expiry_seconds")
	counterExpireTime := contextExpireTime + config.Datadog().GetInt64("dogstatsd_expiry_seconds")

	s := &TimeSampler{
		timeSamplerConst: timeSamplerConst{
			interval: interval,
			id:       id,
			idString: idString,
			hostname: hostname,
		},
		contextResolver:    newTimestampContextResolver(cache, idString, contextExpireTime, counterExpireTime),
		metricsByTimestamp: map[int64]metrics.ContextMetrics{},
		sketchMap:          make(sketchMap),
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
		if err := bucketMetrics.AddSample(contextKey, metricSample, timestamp, s.interval, nil, config.Datadog()); err != nil {
			log.Debugf("TimeSampler #%d Ignoring sample '%s' on host '%s' and tags '%s': %s", s.id, metricSample.Name, metricSample.Host, metricSample.Tags, err)
		}
	}
}

func (s *timeSamplerConst) newSketchSeries(ck ckey.ContextKey, points []metrics.SketchPoint, resolver func(ckey.ContextKey) (*Context, bool)) *metrics.SketchSeries {
	ctx, ok := resolver(ck)
	if !ok {
		return nil
	}

	ss := &metrics.SketchSeries{
		Name:       ctx.Name,
		Tags:       ctx.Tags(),
		Host:       ctx.Host,
		Interval:   s.interval,
		Points:     points,
		ContextKey: ck,
		Source:     ctx.source,
		NoIndex:    ctx.noIndex,
	}

	return ss
}

// splitBefore removes and returns buckets that are closed at the time specified by cutoffTime.
func (s *TimeSampler) splitBefore(cutoffTime int64) metricsMap {
	closed := metricsMap{}
	for bucketTimestamp, contextMetrics := range s.metricsByTimestamp {
		if !s.isBucketStillOpen(bucketTimestamp, cutoffTime) {
			closed[bucketTimestamp] = contextMetrics
			delete(s.metricsByTimestamp, bucketTimestamp)
		}
	}
	return closed
}

func (s *TimeSampler) flushSeries(cutoffTime int64, series metrics.SerieSink) {
	// Map to hold the expired contexts that will need to be deleted after the flush so that we stop sending zeros
	contextMetricsFlusher := metrics.NewContextMetricsFlusher()

	if len(s.metricsByTimestamp) > 0 {
		for bucketTimestamp, contextMetrics := range s.metricsByTimestamp {
			// disregard when the timestamp is too recent
			if s.isBucketStillOpen(bucketTimestamp, cutoffTime) {
				continue
			}

			// Add a 0 sample to all the counters that are not expired.
			// It is ok to add 0 samples to a counter that was already sampled for real in the bucket, since it won't change its value
			s.countersSampleZeroValue(bucketTimestamp, contextMetrics, s.contextResolver.resolver.contextsByKey)
			contextMetricsFlusher.Append(float64(bucketTimestamp), contextMetrics)

			delete(s.metricsByTimestamp, bucketTimestamp)
		}
	} else if s.lastCutOffTime+s.interval <= cutoffTime {
		// Even if there is no metric in this flush, recreate empty counters,
		// but only if we've passed an interval since the last flush

		contextMetrics := metrics.MakeContextMetrics()

		s.countersSampleZeroValue(cutoffTime-s.interval, contextMetrics, s.contextResolver.resolver.contextsByKey)
		contextMetricsFlusher.Append(float64(cutoffTime-s.interval), contextMetrics)
	}

	// serieBySignature is reused for each call of dedupSerieBySerieSignature to avoid allocations.
	serieBySignature := make(map[SerieSignature]*metrics.Serie)
	s.flushContextMetrics(contextMetricsFlusher, func(rawSeries []*metrics.Serie) {
		// Note: rawSeries is reused at each call
		s.dedupSerieBySerieSignature(rawSeries, series, serieBySignature, s.contextResolver.get)
	})
}

func (s *timeSamplerConst) dedupSerieBySerieSignature(
	rawSeries []*metrics.Serie,
	serieSink metrics.SerieSink,
	serieBySignature map[SerieSignature]*metrics.Serie,
	resolver func(ckey.ContextKey) (*Context, bool),
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
			context, ok := resolver(serie.ContextKey)
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
		serieSink.Append(serie)
	}
}

func (s *TimeSampler) flushSketches(cutoffTime int64, sketchesSink metrics.SketchesSink) {
	pointsByCtx := make(map[ckey.ContextKey][]metrics.SketchPoint)

	s.sketchMap.flushBefore(cutoffTime, func(ck ckey.ContextKey, p metrics.SketchPoint) {
		if p.Sketch == nil {
			return
		}
		pointsByCtx[ck] = append(pointsByCtx[ck], p)
	})
	for ck, points := range pointsByCtx {
		ss := s.newSketchSeries(ck, points, s.contextResolver.get)
		if ss == nil {
			log.Errorf("TimeSampler #%d Ignoring all metrics on context key '%v': inconsistent context resolver state: the context is not tracked", s.id, ck)
			continue
		}
		sketchesSink.Append(ss)
	}
}

func (s *TimeSampler) flush(timestamp float64, series metrics.SerieSink, sketches metrics.SketchesSink) {
	// Compute a limit timestamp
	cutoffTime := s.calculateBucketStart(timestamp)

	s.flushSeries(cutoffTime, series)
	s.flushSketches(cutoffTime, sketches)
	// expiring contexts
	s.contextResolver.expireContexts(int64(timestamp))
	s.lastCutOffTime = cutoffTime

	s.updateMetrics()
	s.sendTelemetry(timestamp, s.contextResolver.resolver.contextsByKey, series)
}

func (s *TimeSampler) flushAsync(timestamp float64, series metrics.SerieSink, sketches metrics.SketchesSink, blockChan chan struct{}) {
	// Compute a limit timestamp
	cutoffTime := s.calculateBucketStart(timestamp)
	// Move metrics and sketches buckets that will be flushed out of the active working set into local variables
	metricsBuckets := s.splitBefore(cutoffTime)
	sketchesBuckets := s.sketchMap.splitBefore(cutoffTime)
	contexts := s.contextResolver.cloneContexts()

	s.contextResolver.expireContexts(int64(timestamp))
	s.updateMetrics()

	go s.doFlushAsync(
		timestamp,
		cutoffTime,
		s.lastCutOffTime,
		contexts,
		metricsBuckets,
		sketchesBuckets,
		series,
		sketches,
		blockChan,
	)

	s.lastCutOffTime = cutoffTime
}

// doFlushAsync performs asynchronous flush while time sampler
// is processing new metrics.
//
// This function should not share non-readonly data with other
// goroutines to avoid races.
//
// This isn't a method of TimeSampler to reduce chance of accidentally
// accessing data that is concurrently modified.
func (s *timeSamplerConst) doFlushAsync(
	timestamp float64,
	cutoffTime int64,
	lastCutoffTime int64,
	contexts resolverMap,
	metricsBuckets metricsMap,
	sketchesBuckets sketchMap,
	seriesSink metrics.SerieSink,
	sketchesSink metrics.SketchesSink,
	blockChan chan struct{},
) {
	defer func() { blockChan <- struct{}{} }()

	s.doFlushAsyncMetrics(cutoffTime, lastCutoffTime, contexts, metricsBuckets, seriesSink)
	s.doFlushAsyncSketches(contexts, sketchesBuckets, sketchesSink)
	s.sendTelemetry(timestamp, contexts, seriesSink)
}

func (s *timeSamplerConst) doFlushAsyncMetrics(
	cutoffTime int64,
	lastCutoffTime int64,
	contexts resolverMap,
	metricsBuckets metricsMap,
	seriesSink metrics.SerieSink,
) {
	if len(metricsBuckets) == 0 && lastCutoffTime+s.interval <= cutoffTime {
		// Even if there is no metric in this flush, recreate empty counters,
		// but only if we've passed an interval since the last flush
		metricsBuckets[cutoffTime-s.interval] = metrics.MakeContextMetrics()
	}
	contextMetricsFlusher := metrics.NewContextMetricsFlusher()
	for bucketTimestamp, contextMetrics := range metricsBuckets {
		// Add a 0 sample to all the counters that are not expired. It is ok to
		// add 0 samples to a counter that was already sampled for real in the
		// bucket, since it won't change its value.
		s.countersSampleZeroValue(bucketTimestamp, contextMetrics, contexts)
		contextMetricsFlusher.Append(float64(bucketTimestamp), contextMetrics)
	}

	// serieBySignature is reused for each call of dedupSerieBySerieSignature to avoid allocations.
	serieBySignature := make(map[SerieSignature]*metrics.Serie)
	errors := contextMetricsFlusher.FlushAndClear(func(rawSeries []*metrics.Serie) {
		s.dedupSerieBySerieSignature(rawSeries, seriesSink, serieBySignature, contexts.get)
	})
	for ckey, err := range errors {
		context, ok := contexts.get(ckey)
		if !ok {
			log.Errorf("Can't resolve context of error '%s': inconsistent context resolver state: context with key '%v' is not tracked", err, ckey)
			continue
		}
		log.Infof("No value returned for dogstatsd metric '%s' on host '%s' and tags '%s': %s", context.Name, context.Host, context.Tags(), err)
	}
}

func (s *timeSamplerConst) doFlushAsyncSketches(
	contexts resolverMap,
	sketchesBuckets sketchMap,
	sketchesSink metrics.SketchesSink,
) {
	for ck, points := range sketchesBuckets.toPoints() {
		ss := s.newSketchSeries(ck, points, contexts.get)
		if ss == nil {
			log.Errorf("TimeSampler #%d Ignoring all sketches on context key '%v': inconsistent context resolver state: the context is not tracked", s.id, ck)
			continue
		}
		sketchesSink.Append(ss)
	}
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

func (s *timeSamplerConst) countersSampleZeroValue(timestamp int64, contextMetrics metrics.ContextMetrics, contexts resolverMap) {
	expirySeconds := config.Datadog().GetInt64("dogstatsd_expiry_seconds")
	for counterContext, entry := range contexts {
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
			contextMetrics.AddSample(counterContext, sample, float64(timestamp), s.interval, nil, config.Datadog()) //nolint:errcheck
		}
	}
}

func (s *timeSamplerConst) sendTelemetry(timestamp float64, contexts resolverMap, series metrics.SerieSink) {
	if !config.Datadog().GetBool("telemetry.enabled") {
		return
	}

	// If multiple samplers are used, this avoids the need to
	// aggregate the stats agent-side, and allows us to see amount of
	// tags duplication between shards.
	tags := []string{
		fmt.Sprintf("sampler_id:%d", s.id),
	}

	if config.Datadog().GetBool("telemetry.dogstatsd_origin") {
		contexts.sendOriginTelemetry(timestamp, series, s.hostname, tags)
	}
}

func (s *TimeSampler) dumpContexts(dest io.Writer) error {
	return s.contextResolver.dumpContexts(dest)
}
