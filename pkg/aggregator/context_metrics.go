package aggregator

import (
	"math"

	log "github.com/cihub/seelog"
)

// ContextMetrics stores all the metrics by context key
type ContextMetrics map[string]Metric

func makeContextMetrics() ContextMetrics {
	return ContextMetrics(make(map[string]Metric))
}

// TODO: Pass a reference to *MetricSample instead
func (m ContextMetrics) addSample(contextKey string, sample *MetricSample, timestamp int64, interval int64) {
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

func (m ContextMetrics) flush(timestamp int64) []*Serie {
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
				// this error happens in nominal conditions and shouldn't be logged
			default:
				log.Infof("An error occurred while flushing metric on context key '%s': %s", contextKey, err)
			}
		}
	}

	return series
}
