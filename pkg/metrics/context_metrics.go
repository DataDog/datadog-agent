package metrics

import (
	"math"

	"github.com/DataDog/datadog-agent/pkg/config"
	log "github.com/cihub/seelog"
)

// ContextMetrics stores all the metrics by context key
type ContextMetrics map[string]Metric

var expirySeconds = config.Datadog.GetFloat64("dogstatsd_expiry_seconds")

// MakeContextMetrics returns a new ContextMetrics
func MakeContextMetrics() ContextMetrics {
	return ContextMetrics(make(map[string]Metric))
}

// AddSample add a sample to the current ContextMetrics and initialize a new metrics if needed.
// TODO: Pass a reference to *MetricSample instead
func (m ContextMetrics) AddSample(contextKey string, sample *MetricSample, timestamp float64, interval int64) {
	if math.IsInf(sample.Value, 0) {
		log.Warn("Ignoring sample with +/-Inf value on context key:", contextKey)
		return
	}
	if _, ok := m[contextKey]; !ok {
		switch sample.Mtype {
		case GaugeType:
			m[contextKey] = &Gauge{}
		case RateType:
			m[contextKey] = &Rate{}
		case CountType:
			m[contextKey] = &Count{}
		case MonotonicCountType:
			m[contextKey] = &MonotonicCount{}
		case HistogramType:
			m[contextKey] = &Histogram{} // default histogram configuration for now
		case HistorateType:
			m[contextKey] = &Historate{} // internal histogram have the configuration for now
		case SetType:
			m[contextKey] = NewSet()
		case CounterType:
			m[contextKey] = NewCounter(interval)
		default:
			log.Error("Can't add unknown sample metric type:", sample.Mtype)
			return
		}
	}
	m[contextKey].addSample(sample, timestamp)
}

// Flush flushes every metrics in the ContextMetrics
func (m ContextMetrics) Flush(timestamp float64, samplerMaps ...map[string]float64) []*Serie {
	var series []*Serie
	var counterLastSampledByContext map[string]float64
	var lastSeenByKey map[string]float64

	if len(samplerMaps) == 2 {
		counterLastSampledByContext = samplerMaps[0]
		lastSeenByKey = samplerMaps[1]
	}

	// Copy the map so we can recreate non-expired Counters
	var notSampledInThisBucket = map[string]float64{}
	if counterLastSampledByContext != nil {
		for k, v := range counterLastSampledByContext {
			notSampledInThisBucket[k] = v
		}
	}

	for contextKey, metric := range m {
		if counterLastSampledByContext != nil {
			// Look for non-expired Counter that need to be reported with a 0 value
			if _, isCounter := metric.(*Counter); isCounter {
				// Counter sampled in this bucket
				counterLastSampledByContext[contextKey] = timestamp
				delete(notSampledInThisBucket, contextKey)
			}
		}

		metricSeries, err := metric.flush(timestamp)
		if err == nil {
			for _, serie := range metricSeries {
				serie.ContextKey = contextKey
				series = append(series, serie)
			}
		} else {
			switch err.(type) {
			case NoSerieError:
				// this error happens in nominal conditions and shouldn't be logged
			default:
				log.Infof("An error occurred while flushing metric on context key '%s': %s", contextKey, err)
			}
		}
	}

	// Recreate non-expired Counters and delete expired ones
	for contextKey := range notSampledInThisBucket {
		if expirySeconds+counterLastSampledByContext[contextKey] <= timestamp {
			// Counter expired, stop tracking it
			delete(counterLastSampledByContext, contextKey)
			delete(notSampledInThisBucket, contextKey)
		} else {
			// Create an empty Counter
			emptySerie := &Serie{
				ContextKey: contextKey,
				Points:     []Point{{Ts: timestamp, Value: 0}},
				MType:      APIRateType,
			}
			series = append(series, emptySerie)
			lastSeenByKey[contextKey] = timestamp
		}
	}

	return series
}
