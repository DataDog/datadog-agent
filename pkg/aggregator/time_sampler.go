package aggregator

import (
	"github.com/DataDog/datadog-agent/pkg/config"
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
	// Update LastSampled timestamp for counters
	if metricSample.Mtype == metrics.CounterType {
		s.counterLastSampledByContext[contextKey] = timestamp
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
		for bucketTimestamp, contextMetrics := range s.metricsByTimestamp {
			// disregard when the timestamp is too recent
			if cutoffTime <= bucketTimestamp {
				continue
			}

			s.countersSampleZeroValue(bucketTimestamp, contextMetrics)

			rawSeries = append(rawSeries, contextMetrics.Flush(float64(bucketTimestamp))...)

			delete(s.metricsByTimestamp, bucketTimestamp)
		}
	} else if s.lastCutOffTime+s.interval <= cutoffTime {
		// Even if there is no metric in this flush, recreate empty counters,
		// but only if we've passed an interval since the last flush

		contextMetrics := metrics.MakeContextMetrics()

		s.countersSampleZeroValue(cutoffTime-s.interval, contextMetrics)

		rawSeries = append(rawSeries, contextMetrics.Flush(float64(cutoffTime-s.interval))...)
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

func (s *TimeSampler) countersSampleZeroValue(timestamp int64, contextMetrics metrics.ContextMetrics) {

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
			contextMetrics.AddSample(counterContext, sample, float64(timestamp), s.interval)
			s.contextResolver.updateTrackedContext(counterContext, float64(timestamp))
		} else {
			delete(s.counterLastSampledByContext, counterContext)
		}
	}
}
