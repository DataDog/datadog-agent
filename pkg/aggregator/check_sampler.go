package aggregator

// CheckSampler aggregates metrics from one Check instance
type CheckSampler struct {
	series          []*Serie
	contextResolver *ContextResolver
	metrics         Metrics
	hostname        string
}

// newCheckSampler returns a newly initialized CheckSample
func newCheckSampler(hostname string) *CheckSampler {
	return &CheckSampler{
		series:          make([]*Serie, 0),
		contextResolver: newContextResolver(),
		metrics:         makeMetrics(),
		hostname:        hostname,
	}
}

func (cs *CheckSampler) addSample(metricSample *MetricSample) {
	contextKey := cs.contextResolver.trackContext(metricSample, metricSample.Timestamp)

	cs.metrics.addSample(contextKey, metricSample.Mtype, metricSample.Value, metricSample.Timestamp)
}

func (cs *CheckSampler) commit(timestamp int64) {
	for _, serie := range cs.metrics.flush(timestamp) {
		// Resolve context and populate new []Serie
		context := cs.contextResolver.contextsByKey[serie.contextKey]
		serie.Name = context.Name + serie.nameSuffix
		serie.Tags = context.Tags
		serie.Host = cs.hostname // FIXME: take into account the hostname of the context if it's specified
		serie.DeviceName = context.DeviceName

		cs.series = append(cs.series, serie)
	}

	cs.contextResolver.expireContexts(timestamp - defaultExpirySeconds)
}

func (cs *CheckSampler) flush() []*Serie {
	series := cs.series
	cs.series = make([]*Serie, 0)
	return series
}
