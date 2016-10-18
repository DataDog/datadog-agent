package aggregator

// CheckSampler aggregates metrics from one Check instance
type CheckSampler struct {
	series   []*Serie
	contexts map[string]Context // TODO: this map grows constantly, we need to flush old contexts from time to time. It should also be a shared ContextResolver
	metrics  Metrics
}

// newCheckSampler returns a newly initialized CheckSample
func newCheckSampler() *CheckSampler {
	return &CheckSampler{
		series:   make([]*Serie, 0),
		contexts: map[string]Context{},
		metrics:  *newMetrics(),
	}
}

func (cs *CheckSampler) addSample(metricSample *MetricSample) {
	contextKey := generateContextKey(metricSample)
	if _, ok := cs.contexts[contextKey]; !ok {
		cs.contexts[contextKey] = Context{
			Name:       metricSample.Name,
			Tags:       metricSample.Tags,
			Host:       "",
			DeviceName: "",
		}
	}

	cs.metrics.addSample(contextKey, metricSample.Mtype, metricSample.Value, metricSample.Timestamp)
}

func (cs *CheckSampler) commit(timestamp int64) {
	for _, serie := range cs.metrics.flush(timestamp) {
		// Resolve context and populate new []Serie
		context := cs.contexts[serie.contextKey]
		serie.Name = context.Name + serie.nameSuffix
		serie.Tags = context.Tags
		serie.Host = context.Host
		serie.DeviceName = context.DeviceName

		cs.series = append(cs.series, serie)
	}
}

func (cs *CheckSampler) flush() []*Serie {
	series := cs.series
	cs.series = make([]*Serie, 0)
	return series
}
