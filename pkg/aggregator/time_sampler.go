package aggregator

import (
	"time"
)

const defaultExpiry = 300 * time.Second // duration after which contexts are expired

// SerieSignature holds the elements that allow to know whether two similar `Serie`s
// from the same bucket can be merged into one
type SerieSignature struct {
	mType      APIMetricType
	contextKey string
	nameSuffix string
}

// TimeSampler aggregates metrics by buckets of 'interval' seconds
type TimeSampler struct {
	interval           int64
	contextResolver    *ContextResolver
	metricsByTimestamp map[int64]ContextMetrics
	defaultHostname    string
	reportingCounters  map[string]*Counter
}

// NewTimeSampler returns a newly initialized TimeSampler
func NewTimeSampler(interval int64, defaultHostname string) *TimeSampler {
	return &TimeSampler{
		interval:           interval,
		contextResolver:    newContextResolver(),
		metricsByTimestamp: map[int64]ContextMetrics{},
		defaultHostname:    defaultHostname,
		reportingCounters:  map[string]*Counter{},
	}
}

func (s *TimeSampler) calculateBucketStart(timestamp int64) int64 {
	return timestamp - timestamp%s.interval
}

// Add the metricSample to the correct bucket
func (s *TimeSampler) addSample(metricSample *MetricSample, timestamp int64) {
	// Keep track of the context
	contextKey := s.contextResolver.trackContext(metricSample, timestamp)

	bucketStart := s.calculateBucketStart(timestamp)
	// If it's a new bucket, initialize it
	metrics, ok := s.metricsByTimestamp[bucketStart]
	if !ok {
		metrics = makeContextMetrics()
		s.metricsByTimestamp[bucketStart] = metrics
	}

	// Add sample to bucket
	metrics.addSample(contextKey, metricSample, timestamp, s.interval)

	// If it's a Counter, keep track of it to report 0 values when no data
	if metricSample.Mtype == CounterType {
		s.reportingCounters[contextKey] = metrics[contextKey].(*Counter)
	}
}

func (s *TimeSampler) flush(timestamp int64) []*Serie {
	var result []*Serie
	var rawSeries []*Serie

	serieBySignature := make(map[SerieSignature]*Serie)

	// Compute a limit timestamp
	cutoffTime := s.calculateBucketStart(timestamp)

	// Flush all counters that were not sampled so that they report a 0 value
	rawSeries = append(rawSeries, s.flushNotSampledCounters(cutoffTime-s.interval)...)

	// Iter on each bucket
	for timestamp, metrics := range s.metricsByTimestamp {
		// disregard when the timestamp is too recent
		if cutoffTime <= timestamp {
			continue
		}

		rawSeries = append(rawSeries, metrics.flush(timestamp)...)

		delete(s.metricsByTimestamp, timestamp)
	}

	for _, serie := range rawSeries {
		serieSignature := SerieSignature{serie.MType, serie.contextKey, serie.nameSuffix}

		if existingSerie, ok := serieBySignature[serieSignature]; ok {
			existingSerie.Points = append(existingSerie.Points, serie.Points[0])
		} else {
			// Resolve context and populate new Serie
			context := s.contextResolver.contextsByKey[serie.contextKey]
			serie.Name = context.Name + serie.nameSuffix
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

	s.contextResolver.expireContexts(timestamp - int64(defaultExpiry/time.Second))

	return result
}

func (s *TimeSampler) flushNotSampledCounters(timestamp int64) []*Serie {
	contextMetrics := makeContextMetrics()

	for contextKey, counter := range s.reportingCounters {
		// If the counter has not been sampled for too long, stop tracking it
		if counter.expiration <= timestamp {
			delete(s.reportingCounters, contextKey)
			continue
		}
		// If no sample was added between two flushes to the counter, it was not flushed
		// Add it to the ContextMetrics to be flushed and report a 0 value
		if !counter.sampled {
			contextMetrics[contextKey] = counter
			// Keep tracking the context
			s.contextResolver.lastSeenByKey[contextKey] = timestamp
		}
	}
	return contextMetrics.flush(timestamp)
}
