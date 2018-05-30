// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package aggregator

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

const defaultExpiry = 300.0 // number of seconds after which contexts are expired

// SerieSignature holds the elements that allow to know whether two similar `Serie`s
// from the same bucket can be merged into one
type SerieSignature struct {
	mType      metrics.APIMetricType
	contextKey ckey.ContextKey
	nameSuffix string
}

// TimeSampler aggregates metrics by buckets of 'interval' seconds
type TimeSampler struct {
	interval                    int64
	contextResolver             *ContextResolver
	metricsByTimestamp          map[int64]metrics.ContextMetrics
	defaultHostname             string
	counterLastSampledByContext map[ckey.ContextKey]float64
	lastCutOffTime              int64
}

// NewTimeSampler returns a newly initialized TimeSampler
func NewTimeSampler(interval int64, defaultHostname string) *TimeSampler {
	return &TimeSampler{
		interval:                    interval,
		contextResolver:             newContextResolver(),
		metricsByTimestamp:          map[int64]metrics.ContextMetrics{},
		defaultHostname:             defaultHostname,
		counterLastSampledByContext: map[ckey.ContextKey]float64{},
	}
}

func (s *TimeSampler) calculateBucketStart(timestamp float64) int64 {
	return int64(timestamp) - int64(timestamp)%s.interval
}

// Add the metricSample to the correct bucket
func (s *TimeSampler) addSample(metricSample *metrics.MetricSample, timestamp float64) {
	// Keep track of the context
	contextKey := s.contextResolver.trackContext(metricSample, timestamp)

	bucketStart := s.calculateBucketStart(timestamp)
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
	if err := bucketMetrics.AddSample(contextKey, metricSample, timestamp, s.interval); err != nil {
		log.Debug("Ignoring sample '%s' on host '%s' and tags '%s': %s", metricSample.Name, metricSample.Host, metricSample.Tags, err)
	}
}

func (s *TimeSampler) flush(timestamp float64) metrics.Series {
	var result []*metrics.Serie
	var rawSeries []*metrics.Serie

	serieBySignature := make(map[SerieSignature]*metrics.Serie)

	// Compute a limit timestamp
	cutoffTime := s.calculateBucketStart(timestamp)

	// Map to hold the expired contexts that will need to be deleted after the flush so that we stop sending zeros
	counterContextsToDelete := map[ckey.ContextKey]struct{}{}

	if len(s.metricsByTimestamp) > 0 {
		for bucketTimestamp, contextMetrics := range s.metricsByTimestamp {
			// disregard when the timestamp is too recent
			if cutoffTime <= bucketTimestamp {
				continue
			}

			// Add a 0 sample to all the counters that are not expired.
			// It is ok to add 0 samples to a counter that was already sampled for real in the bucket, since it won't change its value
			s.countersSampleZeroValue(bucketTimestamp, contextMetrics, counterContextsToDelete)

			rawSeries = append(rawSeries, s.flushContextMetrics(bucketTimestamp, contextMetrics)...)

			delete(s.metricsByTimestamp, bucketTimestamp)
		}
	} else if s.lastCutOffTime+s.interval <= cutoffTime {
		// Even if there is no metric in this flush, recreate empty counters,
		// but only if we've passed an interval since the last flush

		contextMetrics := metrics.MakeContextMetrics()

		s.countersSampleZeroValue(cutoffTime-s.interval, contextMetrics, counterContextsToDelete)

		rawSeries = append(rawSeries, s.flushContextMetrics(cutoffTime-s.interval, contextMetrics)...)
	}

	// Delete the contexts associated to an expired counter
	for context := range counterContextsToDelete {
		delete(s.counterLastSampledByContext, context)
	}

	for _, serie := range rawSeries {
		serieSignature := SerieSignature{serie.MType, serie.ContextKey, serie.NameSuffix}

		if existingSerie, ok := serieBySignature[serieSignature]; ok {
			existingSerie.Points = append(existingSerie.Points, serie.Points[0])
		} else {
			// Resolve context and populate new Serie
			context, ok := s.contextResolver.contextsByKey[serie.ContextKey]
			if !ok {
				log.Errorf("Ignoring all metrics on context key '%v': inconsistent context resolver state: the context is not tracked", serie.ContextKey)
				continue
			}
			serie.Name = context.Name + serie.NameSuffix
			serie.Tags = context.Tags
			if context.Host != "" {
				serie.Host = context.Host
			} else {
				serie.Host = s.defaultHostname
			}
			serie.Interval = s.interval

			serieBySignature[serieSignature] = serie
			result = append(result, serie)
		}
	}

	s.contextResolver.expireContexts(timestamp - defaultExpiry)
	s.lastCutOffTime = cutoffTime
	return result
}

// flushContextMetrics flushes the passed contextMetrics, handles its errors, and returns its series
func (s *TimeSampler) flushContextMetrics(timestamp int64, contextMetrics metrics.ContextMetrics) []*metrics.Serie {
	series, errors := contextMetrics.Flush(float64(timestamp))
	for ckey, err := range errors {
		context, ok := s.contextResolver.contextsByKey[ckey]
		if !ok {
			log.Errorf("Can't resolve context of error '%s': inconsistent context resolver state: context with key '%v' is not tracked", err, ckey)
			continue
		}
		log.Infof("No value returned for dogstatsd metric '%s' on host '%s' and tags '%s': %s", context.Name, context.Host, context.Tags, err)
	}
	return series
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
			contextMetrics.AddSample(counterContext, sample, float64(timestamp), s.interval)

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
