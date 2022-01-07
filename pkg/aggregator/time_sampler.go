// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/aggregator/tags"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer"
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

// TimeSampler aggregates metrics by buckets of 'interval' seconds
type TimeSampler struct {
	interval                    int64
	contextResolver             *timestampContextResolver
	metricsByTimestamp          map[int64]metrics.ContextMetrics
	counterLastSampledByContext map[ckey.ContextKey]float64
	lastCutOffTime              int64
	sketchMap                   sketchMap

	// pointer to the shared MetricSamplePool stored in the Demultiplexer.
	metricSamplePool *metrics.MetricSamplePool

	// id is a number to differentiate multiple time samplers
	// since we start running more than one with the demultiplexer introduction
	id         TimeSamplerID
	serializer serializer.MetricSerializer
	stopChan   chan struct{}

	// samples channel used to communicate from the calling routine to the one
	// actively processing the samples
	samples chan []metrics.MetricSample

	// use this chan to command a flush of the time sampler
	FlushChan chan FlushCommand

	// parallel serialization configuration
	parallelSerialization flushAndSerializeInParallel
}

// FlushCommand must be use to execute a flush of the TimeSampler.
// If `BlockChan` is not nil, a message is sent when the flush is complete.
type FlushCommand struct {
	Time      time.Time
	BlockChan chan struct{}
}

// NewTimeSampler returns a newly initialized TimeSampler
func NewTimeSampler(id TimeSamplerID, interval int64, metricSamplePool *metrics.MetricSamplePool, bufferSize int, serializer serializer.MetricSerializer, cache *tags.Store, parallelSerialization flushAndSerializeInParallel) *TimeSampler {
	if interval == 0 {
		interval = bucketSize
	}

	log.Infof("Creating TimeSampler #%d", id)

	ts := &TimeSampler{
		interval:                    interval,
		contextResolver:             newTimestampContextResolver(cache),
		metricsByTimestamp:          map[int64]metrics.ContextMetrics{},
		counterLastSampledByContext: map[ckey.ContextKey]float64{},
		sketchMap:                   make(sketchMap),
		id:                          id,
		stopChan:                    make(chan struct{}),
		serializer:                  serializer,
		metricSamplePool:            metricSamplePool,
		samples:                     make(chan []metrics.MetricSample, bufferSize),
		FlushChan:                   make(chan FlushCommand),
		parallelSerialization:       parallelSerialization,
	}

	go ts.processLoop(time.Second * time.Duration(interval))

	return ts
}

func (s *TimeSampler) calculateBucketStart(timestamp float64) int64 {
	return int64(timestamp) - int64(timestamp)%s.interval
}

func (s *TimeSampler) isBucketStillOpen(bucketStartTimestamp, timestamp int64) bool {
	return bucketStartTimestamp+s.interval > timestamp
}

// Add the metricSample to the correct bucket
func (s *TimeSampler) addSamples(samples []metrics.MetricSample) {
	s.samples <- samples
}

func (s *TimeSampler) sample(metricSample *metrics.MetricSample, timestamp float64) {
	// use the timestamp provided in the sample if any
	if metricSample.Timestamp > 0 {
		timestamp = metricSample.Timestamp
	}

	// Keep track of the context
	contextKey := s.contextResolver.trackContext(metricSample, timestamp)
	bucketStart := s.calculateBucketStart(timestamp)

	// log.Infof("TimeSampler #%d processed sample '%s' tags '%s': %s", s.id, metricSample.Name, metricSample.Tags)

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
		// Update LastSampled timestamp for counters
		if metricSample.Mtype == metrics.CounterType {
			s.counterLastSampledByContext[contextKey] = timestamp
		}

		// Add sample to bucket
		if err := bucketMetrics.AddSample(contextKey, metricSample, timestamp, s.interval, nil); err != nil {
			log.Debugf("TimeSampler #%d Ignoring sample '%s' on host '%s' and tags '%s': %s", s.id, metricSample.Name, metricSample.Host, metricSample.Tags, err)
		}
	}
}

// Stop stops the time sampler. It can't be re-used after being stop,
// use NewTimeSampler instead.
func (s *TimeSampler) Stop() {
	s.stopChan <- struct{}{}
}

// We process all receivend samples in the `select`, but we also process a flush action,
// meaning that the time sampler will not process any sample while it is flushing.
// Note that it was the same design in the BufferedAggregator (but at the aggregator level,
// not sampler level).
// If we want to move to a design where we can flush while we are processing samples,
// we could consider implementing double-buffering or locking for every sample reception.
func (s *TimeSampler) processLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	for {
		select {
		case <-s.stopChan:
			return
		case ms := <-s.samples:
			// do this telemetry here and not in the samplers goroutines, this way
			// they won't compete for the telemetry locks.
			aggregatorDogstatsdMetricSample.Add(int64(len(ms)))
			tlmProcessed.Add(float64(len(ms)), "dogstatsd_metrics")
			t := timeNowNano()
			for i := 0; i < len(ms); i++ {
				s.sample(&ms[i], t)
			}
			s.metricSamplePool.PutBatch(ms)
		case command := <-s.FlushChan:
			s.triggerFlush(command.Time, command.BlockChan != nil)
			if command.BlockChan != nil {
				command.BlockChan <- struct{}{}
			}
		case t := <-ticker.C:
			s.triggerFlush(t, false)
		}
	}
}

func (s *TimeSampler) triggerFlush(t time.Time, waitForSerializer bool) {
	if s.parallelSerialization.enabled {
		s.triggerFlushWithParallelSerialize(t, waitForSerializer)
	} else {
		log.Debugf("Time Sampler #%d Flushing series to the forwarder", s.id)
		var series metrics.Series
		sketches := s.flush(float64(t.Unix()), &series) // XXX(remy): is this conversation correct? note that it is in second
		// XXX(remy): better error management
		if s.serializer != nil {
			//
			// TODO(remy): restore all the telemetry
			//
			if err := s.serializer.SendSeries(series); err != nil {
				log.Errorf("flushLoop: %+v", err)
			}
			tagsetTlm.updateHugeSeriesTelemetry(&series)

			if err := s.serializer.SendSketch(sketches); err != nil {
				log.Errorf("flushLoop: %+v", err)
			}
			tagsetTlm.updateHugeSketchesTelemetry(&sketches)
		}
	}
}

// NOTE(remy): this has been stolen from the Aggregator implementation, we will have
// to factor it at some point.
func (s *TimeSampler) sendIterableSeries(
	start time.Time,
	series *metrics.IterableSeries,
	done chan<- struct{}) {
	go func() {
		log.Debugf("Time Sampler #%d Flushing series to the forwarder in parallel", s.id)

		err := s.serializer.SendIterableSeries(series)
		// if err == nil, SenderStopped was called and it is safe to read the number of series.
		count := series.SeriesCount()
		addFlushCount("Series", int64(count))
		updateSerieTelemetry(start, int(count), err)
		close(done)
	}()
}

// NOTE(remy): this has been stolen from the Aggregator implementation, we will have
// to factor it at some point.
func (s *TimeSampler) triggerFlushWithParallelSerialize(start time.Time, waitForSerializer bool) {
	logPayloads := config.Datadog.GetBool("log_payloads")
	series := metrics.NewIterableSeries(func(se *metrics.Serie) {
		if logPayloads {
			log.Debugf("Time Sampler #%d Flushing the following metrics: %s", s.id, se)
		}
		tagsetTlm.updateHugeSerieTelemetry(se)
	}, s.parallelSerialization.channelSize, s.parallelSerialization.bufferSize)
	done := make(chan struct{})

	// start the serialization routine
	s.sendIterableSeries(start, series, done)

	sketches := s.flush(float64(start.Unix()), series)
	series.SenderStopped()

	if waitForSerializer {
		<-done
	}

	tagsetTlm.updateHugeSketchesTelemetry(&sketches)
	if err := s.serializer.SendSketch(sketches); err != nil {
		log.Errorf("flushLoop: %+v", err)
	}
}

func (s *TimeSampler) newSketchSeries(ck ckey.ContextKey, points []metrics.SketchPoint) metrics.SketchSeries {
	ctx, _ := s.contextResolver.get(ck)
	ss := metrics.SketchSeries{
		Name:       ctx.Name,
		Tags:       ctx.Tags(),
		Host:       ctx.Host,
		Interval:   s.interval,
		Points:     points,
		ContextKey: ck,
	}

	return ss
}

func (s *TimeSampler) flushSeries(cutoffTime int64, series metrics.SerieSink) {
	// Map to hold the expired contexts that will need to be deleted after the flush so that we stop sending zeros
	counterContextsToDelete := map[ckey.ContextKey]struct{}{}
	contextMetricsFlusher := metrics.NewContextMetricsFlusher()

	if len(s.metricsByTimestamp) > 0 {
		for bucketTimestamp, contextMetrics := range s.metricsByTimestamp {
			// disregard when the timestamp is too recent
			if s.isBucketStillOpen(bucketTimestamp, cutoffTime) {
				continue
			}

			// Add a 0 sample to all the counters that are not expired.
			// It is ok to add 0 samples to a counter that was already sampled for real in the bucket, since it won't change its value
			s.countersSampleZeroValue(bucketTimestamp, contextMetrics, counterContextsToDelete)
			contextMetricsFlusher.Append(float64(bucketTimestamp), contextMetrics)

			delete(s.metricsByTimestamp, bucketTimestamp)
		}
	} else if s.lastCutOffTime+s.interval <= cutoffTime {
		// Even if there is no metric in this flush, recreate empty counters,
		// but only if we've passed an interval since the last flush

		contextMetrics := metrics.MakeContextMetrics()

		s.countersSampleZeroValue(cutoffTime-s.interval, contextMetrics, counterContextsToDelete)
		contextMetricsFlusher.Append(float64(cutoffTime-s.interval), contextMetrics)
	}

	// serieBySignature is reused for each call of dedupSerieBySerieSignature to avoid allocations.
	serieBySignature := make(map[SerieSignature]*metrics.Serie)
	s.flushContextMetrics(contextMetricsFlusher, func(rawSeries []*metrics.Serie) {
		// Note: rawSeries is reused at each call
		s.dedupSerieBySerieSignature(rawSeries, series, serieBySignature)
	})

	// Delete the contexts associated to an expired counter
	for context := range counterContextsToDelete {
		delete(s.counterLastSampledByContext, context)
	}
}

func (s *TimeSampler) dedupSerieBySerieSignature(
	rawSeries []*metrics.Serie,
	serieSink metrics.SerieSink,
	serieBySignature map[SerieSignature]*metrics.Serie) {

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
			serie.Interval = s.interval

			serieBySignature[serieSignature] = serie
		}
	}

	for _, serie := range serieBySignature {
		serieSink.Append(serie)
	}
}

func (s *TimeSampler) flushSketches(cutoffTime int64) metrics.SketchSeriesList {
	pointsByCtx := make(map[ckey.ContextKey][]metrics.SketchPoint)
	sketches := make(metrics.SketchSeriesList, 0, len(pointsByCtx))

	s.sketchMap.flushBefore(cutoffTime, func(ck ckey.ContextKey, p metrics.SketchPoint) {
		if p.Sketch == nil {
			return
		}
		pointsByCtx[ck] = append(pointsByCtx[ck], p)
	})
	for ck, points := range pointsByCtx {
		sketches = append(sketches, s.newSketchSeries(ck, points))
	}

	return sketches
}

func (s *TimeSampler) flush(timestamp float64, series metrics.SerieSink) metrics.SketchSeriesList {
	// Compute a limit timestamp
	cutoffTime := s.calculateBucketStart(timestamp)

	s.flushSeries(cutoffTime, series)
	sketches := s.flushSketches(cutoffTime)

	// expiring contexts
	s.contextResolver.expireContexts(timestamp - config.Datadog.GetFloat64("dogstatsd_context_expiry_seconds"))
	s.lastCutOffTime = cutoffTime

	aggregatorDogstatsdContexts.Set(int64(s.contextResolver.length()))
	tlmDogstatsdContexts.Set(float64(s.contextResolver.length()))
	return sketches
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

func (s *TimeSampler) countersSampleZeroValue(timestamp int64, contextMetrics metrics.ContextMetrics, counterContextsToDelete map[ckey.ContextKey]struct{}) {
	expirySeconds := config.Datadog.GetFloat64("dogstatsd_expiry_seconds")
	for counterContext, lastSampled := range s.counterLastSampledByContext {
		if expirySeconds+lastSampled > float64(timestamp) {
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
			contextMetrics.AddSample(counterContext, sample, float64(timestamp), s.interval, nil) //nolint:errcheck

			// Update the tracked context so that the contextResolver doesn't expire counter contexts too early
			// i.e. while we are still sending zeros for them
			err := s.contextResolver.updateTrackedContext(counterContext, float64(timestamp))
			if err != nil {
				log.Errorf("Error updating context: %s", err)
			}
		} else {
			// Register the context to be deleted
			counterContextsToDelete[counterContext] = struct{}{}
		}
	}
}
