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
}

// NewTimeSampler returns a newly initialized TimeSampler
func NewTimeSampler(interval int64, defaultHostname string) *TimeSampler {
	return &TimeSampler{
		interval:           interval,
		contextResolver:    newContextResolver(),
		metricsByTimestamp: map[int64]ContextMetrics{},
		defaultHostname:    defaultHostname,
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
}

func (s *TimeSampler) flush(timestamp int64) []*Serie {
	var result []*Serie

	serieBySignature := make(map[SerieSignature]*Serie)

	// Compute a limit timestamp
	cutoffTime := s.calculateBucketStart(timestamp)

	// Iter on each bucket
	for timestamp, metrics := range s.metricsByTimestamp {
		// disregard when the timestamp is too recent
		if cutoffTime <= timestamp {
			continue
		}

		series := metrics.flush(timestamp)
		for _, serie := range series {
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

		delete(s.metricsByTimestamp, timestamp)
	}

	s.contextResolver.expireContexts(timestamp - int64(defaultExpiry/time.Second))

	return result
}
