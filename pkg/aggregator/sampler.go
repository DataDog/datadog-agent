package aggregator

import log "github.com/cihub/seelog"

const defaultExpirySeconds = 300 // duration in seconds after which contexts are expired

// Metrics stores all the metrics by context key
type Metrics map[string]Metric

func makeMetrics() Metrics {
	return Metrics(make(map[string]Metric))
}

// SerieSignature holds the elements that allow to know whether two similar `Serie`s
// from the same bucket can be merged into one
type SerieSignature struct {
	mType      APIMetricType
	contextKey string
	nameSuffix string
}

// Sampler aggregates metrics
type Sampler struct {
	interval           int64
	contextResolver    *ContextResolver
	metricsByTimestamp map[int64]Metrics
}

// NewSampler returns a newly initialized Sampler
func NewSampler(interval int64) *Sampler {
	return &Sampler{interval, newContextResolver(), map[int64]Metrics{}}
}

func (s *Sampler) calculateBucketStart(timestamp int64) int64 {
	return timestamp - timestamp%s.interval
}

// Add the metricSample to the correct bucket
func (s *Sampler) addSample(metricSample *MetricSample, timestamp int64) {
	// Keep track of the context
	contextKey := s.contextResolver.trackContext(metricSample, timestamp)

	bucketStart := s.calculateBucketStart(timestamp)
	// If it's a new bucket, initialize it
	metrics, ok := s.metricsByTimestamp[bucketStart]
	if !ok {
		metrics = makeMetrics()
		s.metricsByTimestamp[bucketStart] = metrics
	}

	// Add sample to bucket
	metrics.addSample(contextKey, metricSample.Mtype, metricSample.Value, timestamp)
}

func (s *Sampler) flush(timestamp int64) []*Serie {
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
				serie.Host = context.Host
				serie.DeviceName = context.DeviceName
				serie.Interval = s.interval

				serieBySignature[serieSignature] = serie
				result = append(result, serie)
			}
		}

		delete(s.metricsByTimestamp, timestamp)
	}

	s.contextResolver.expireContexts(timestamp - defaultExpirySeconds)

	return result
}

// TODO: Pass a reference to *MetricSample instead
func (m Metrics) addSample(contextKey string, mType MetricType, value float64, timestamp int64) {
	if _, ok := m[contextKey]; !ok {
		switch mType {
		case GaugeType:
			m[contextKey] = &Gauge{}
		case RateType:
			m[contextKey] = &Rate{}
		case HistogramType:
			m[contextKey] = &Histogram{} // default histogram configuration for now
		default:
			log.Error("Can't add unknown sample metric type:", mType)
			return
		}
	}
	m[contextKey].addSample(value, timestamp)
}

func (m Metrics) flush(timestamp int64) []*Serie {
	var series []*Serie

	for contextKey, metric := range m {
		metricSeries, err := metric.flush(timestamp)

		if err == nil {
			for _, serie := range metricSeries {
				serie.contextKey = contextKey
				series = append(series, serie)
			}
		} else {
			switch err.(type) {
			case NoSerieError:
				log.Debugf("%s on context key %s", err, contextKey)
			default:
				log.Info(err)
			}
		}
	}

	return series
}
