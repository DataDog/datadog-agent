package aggregator

import (
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

const defaultExpiry = 300.0 // number of seconds after which contexts are expired

// SerieSignature holds the elements that allow to know whether two similar `Serie`s
// from the same bucket can be merged into one
type SerieSignature struct {
	mType      metrics.APIMetricType
	contextKey string
	nameSuffix string
}

// TimeSampler aggregates metrics by buckets of 'interval' seconds
type TimeSampler struct {
	interval           int64
	contextResolver    *ContextResolver
	metricsByTimestamp map[int64]metrics.ContextMetrics
	defaultHostname    string
}

// NewTimeSampler returns a newly initialized TimeSampler
func NewTimeSampler(interval int64, defaultHostname string) *TimeSampler {
	return &TimeSampler{
		interval:           interval,
		contextResolver:    newContextResolver(),
		metricsByTimestamp: map[int64]metrics.ContextMetrics{},
		defaultHostname:    defaultHostname,
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

	// Add sample to bucket
	bucketMetrics.AddSample(contextKey, metricSample, timestamp, s.interval)
}

func (s *TimeSampler) flush(timestamp float64) metrics.Series {
	var result []*metrics.Serie

	serieBySignature := make(map[SerieSignature]*metrics.Serie)

	// Compute a limit timestamp
	cutoffTime := s.calculateBucketStart(timestamp)

	// Iter on each bucket
	for bucketTimestamp, metrics := range s.metricsByTimestamp {
		// disregard when the timestamp is too recent
		if cutoffTime <= bucketTimestamp {
			continue
		}

		series := metrics.Flush(float64(bucketTimestamp))
		for _, serie := range series {
			serieSignature := SerieSignature{serie.MType, serie.ContextKey, serie.NameSuffix}

			if existingSerie, ok := serieBySignature[serieSignature]; ok {
				existingSerie.Points = append(existingSerie.Points, serie.Points[0])
			} else {
				// Resolve context and populate new Serie
				context := s.contextResolver.contextsByKey[serie.ContextKey]
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

		delete(s.metricsByTimestamp, bucketTimestamp)
	}

	s.contextResolver.expireContexts(timestamp - defaultExpiry)

	return result
}
