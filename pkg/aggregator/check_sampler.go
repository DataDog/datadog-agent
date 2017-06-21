package aggregator

import "time"

const checksSourceTypeName = "System"

// CheckSampler aggregates metrics from one Check instance
type CheckSampler struct {
	series          []*Serie
	contextResolver *ContextResolver
	metrics         ContextMetrics
	defaultHostname string
}

// newCheckSampler returns a newly initialized CheckSampler
func newCheckSampler(hostname string) *CheckSampler {
	return &CheckSampler{
		series:          make([]*Serie, 0),
		contextResolver: newContextResolver(),
		metrics:         makeContextMetrics(),
		defaultHostname: hostname,
	}
}

func (cs *CheckSampler) addSample(metricSample *MetricSample) {
	contextKey := cs.contextResolver.trackContext(metricSample, metricSample.Timestamp)

	cs.metrics.addSample(contextKey, metricSample, metricSample.Timestamp, 1)
}

func (cs *CheckSampler) commit(timestamp int64) {
	for _, serie := range cs.metrics.flush(timestamp) {
		// Resolve context and populate new []Serie
		context := cs.contextResolver.contextsByKey[serie.contextKey]
		serie.Name = context.Name + serie.nameSuffix
		serie.Tags = context.Tags
		serie.SourceTypeName = checksSourceTypeName // this source type is required for metrics coming from the checks
		if context.Host != "" {
			serie.Host = context.Host
		} else {
			serie.Host = cs.defaultHostname
		}

		cs.series = append(cs.series, serie)
	}

	cs.contextResolver.expireContexts(timestamp - int64(defaultExpiry/time.Second))
}

func (cs *CheckSampler) flush() []*Serie {
	series := cs.series
	cs.series = make([]*Serie, 0)
	return series
}
