package aggregator

import (
	"sort"

	"github.com/DataDog/datadog-agent/pkg/metrics"
)

const defaultExpiry = 300.0 // number of seconds after which contexts are expired

// Wrapper for sorting an int64 array
type int64s []int64

func (a int64s) Less(i, j int) bool {
	return a[i] < a[j]
}
func (a int64s) Len() int {
	return len(a)
}
func (a int64s) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}

// SerieSignature holds the elements that allow to know whether two similar `Serie`s
// from the same bucket can be merged into one
type SerieSignature struct {
	mType      metrics.APIMetricType
	contextKey string
	nameSuffix string
}

// TimeSampler aggregates metrics by buckets of 'interval' seconds
type TimeSampler struct {
	interval                    int64
	contextResolver             *ContextResolver
	metricsByTimestamp          map[int64]metrics.ContextMetrics
	defaultHostname             string
	counterLastSampledByContext map[string]float64
	lastCutOffTime              int64
}

// NewTimeSampler returns a newly initialized TimeSampler
func NewTimeSampler(interval int64, defaultHostname string) *TimeSampler {
	return &TimeSampler{
		interval:                    interval,
		contextResolver:             newContextResolver(),
		metricsByTimestamp:          map[int64]metrics.ContextMetrics{},
		defaultHostname:             defaultHostname,
		counterLastSampledByContext: map[string]float64{},
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

func (s *TimeSampler) flush(timestamp float64) []*metrics.Serie {
	var result []*metrics.Serie
	var rawSeries []*metrics.Serie

	serieBySignature := make(map[SerieSignature]*metrics.Serie)

	// Compute a limit timestamp
	cutoffTime := s.calculateBucketStart(timestamp)

	if len(s.metricsByTimestamp) > 0 {
		// Create an array of the timestamp keys and sort them to iterate through the map in order
		var orderedTimestamps int64s
		for timestamp := range s.metricsByTimestamp {
			orderedTimestamps = append(orderedTimestamps, timestamp)
		}
		sort.Sort(orderedTimestamps)

		// Iter on each bucket in order
		for _, timestamp := range orderedTimestamps {
			metrics := s.metricsByTimestamp[timestamp]
			// disregard when the timestamp is too recent
			if cutoffTime <= timestamp {
				continue
			}

			rawSeries = append(rawSeries, metrics.Flush(float64(timestamp), s.counterLastSampledByContext, s.contextResolver.lastSeenByKey)...)

			delete(s.metricsByTimestamp, timestamp)
		}
	} else if s.lastCutOffTime+s.interval <= cutoffTime {
		// Even if there is no metric in this flush, recreate empty counters,
		// but only if we've passed an interval since the last flush
		rawSeries = append(rawSeries, metrics.MakeContextMetrics().Flush(timestamp, s.counterLastSampledByContext, s.contextResolver.lastSeenByKey)...)
	}

	for _, serie := range rawSeries {
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

	s.contextResolver.expireContexts(timestamp - defaultExpiry)
	s.lastCutOffTime = cutoffTime
	return result
}
