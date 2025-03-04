// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sampler

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"
	"github.com/DataDog/datadog-go/v5/statsd"
)

const (
	// MetricSamplerSeen is the metric name for the number of traces seen by the sampler.
	MetricSamplerSeen = "datadog.trace_agent.sampler.seen"
	// MetricSamplerKept is the metric name for the number of traces kept by the sampler.
	MetricSamplerKept = "datadog.trace_agent.sampler.kept"
	// MetricSamplerSize is the current number of unique trace signatures tracked for stats calculation.
	MetricSamplerSize = "datadog.trace_agent.sampler.size"
)

// Name represents the name of the sampler.
type Name uint8

const (
	// NameUnknown is the default value. It should not be used.
	NameUnknown Name = iota
	// NamePriority is the name of the priority sampler.
	NamePriority
	// NameNoPriority is the name of the no priority sampler.
	NameNoPriority
	// NameError is the name of the error sampler.
	NameError
	// NameRare is the name of the rare sampler.
	NameRare
	// NameProbabilistic is the name of the probabilistic sampler.
	NameProbabilistic
)

// String returns the string representation of the Name.
func (n Name) String() string {
	switch n {
	case NamePriority:
		return "priority"
	case NameNoPriority:
		return "no_priority"
	case NameError:
		return "error"
	case NameRare:
		return "rare"
	case NameProbabilistic:
		return "probabilistic"
	default:
		return "unknown"
	}
}

func (n Name) shouldAddEnvTag() bool {
	return n == NamePriority || n == NameNoPriority || n == NameRare || n == NameError
}

// Metrics is a structure to record metrics for the different samplers.
type Metrics struct {
	statsd              statsd.ClientInterface
	valueMutex          sync.Mutex
	value               map[MetricsKey]metricsValue
	additionalReporters []AdditionalMetricsReporter
	startMutex          sync.Mutex
	ticker              *time.Ticker
	started             bool
}

type metricsValue struct {
	seen int64
	kept int64
}

// NewMetrics creates a new Metrics.
func NewMetrics(statsd statsd.ClientInterface) *Metrics {
	return &Metrics{
		statsd: statsd,
		value:  make(map[MetricsKey]metricsValue),
	}
}

// AdditionalMetricsReporter reports additional sampler metrics.
// Metrics reported through this interface are reported at each Metrics tick.
type AdditionalMetricsReporter interface {
	report(statsd statsd.ClientInterface)
}

// Add sampler metrics reporter.
func (m *Metrics) Add(mr ...AdditionalMetricsReporter) {
	m.additionalReporters = append(m.additionalReporters, mr...)
}

// MetricsKey represents the key for the metrics.
type MetricsKey struct {
	targetService    string
	targetEnv        string
	samplingPriority SamplingPriority
	sampler          Name
}

// NewMetricsKey creates a new MetricsKey.
func NewMetricsKey(service, env string, sampler Name, samplingPriority SamplingPriority) MetricsKey {
	mk := MetricsKey{
		targetService: service,
		targetEnv:     env,
		sampler:       sampler,
	}
	if sampler == NamePriority {
		mk.samplingPriority = samplingPriority
	}
	return mk
}

func (k MetricsKey) tags() []string {
	tags := make([]string, 0, 4) // Pre-allocate number of fields for efficiency
	tags = append(tags, "sampler:"+k.sampler.String())
	if k.sampler == NamePriority {
		tags = append(tags, "sampling_priority:"+k.samplingPriority.tagValue())
	}
	if k.targetService != "" {
		tags = append(tags, "target_service:"+k.targetService)
	}
	if k.targetEnv != "" && k.sampler.shouldAddEnvTag() {
		tags = append(tags, "target_env:"+k.targetEnv)
	}
	return tags
}

// RecordMetricsKey records if metricsKey has been seen before and whether it was kept or not.
func (m *Metrics) RecordMetricsKey(sampled bool, metricsKey MetricsKey) {
	m.valueMutex.Lock()
	defer m.valueMutex.Unlock()
	v, ok := m.value[metricsKey]
	if !ok {
		mv := metricsValue{seen: 1}
		if sampled {
			mv.kept = 1
		}
		m.value[metricsKey] = mv
		return
	}
	v.seen++
	if sampled {
		v.kept++
	}
	m.value[metricsKey] = v
}

// Start the metrics reporting loop.
func (m *Metrics) Start() {
	m.startMutex.Lock()
	defer m.startMutex.Unlock()
	if m.started {
		return
	}
	m.started = true
	m.ticker = time.NewTicker(10 * time.Second)
	go func() {
		defer watchdog.LogOnPanic(m.statsd)
		for range m.ticker.C {
			m.Report()
		}
	}()
}

// Stop the metrics reporting loop.
func (m *Metrics) Stop() {
	m.startMutex.Lock()
	if !m.started {
		m.startMutex.Unlock()
		return
	}
	m.started = false
	m.ticker.Stop()
	m.startMutex.Unlock()
	m.Report()
}

// Report reports the metrics and additional sampler metrics.
func (m *Metrics) Report() {
	m.valueMutex.Lock()
	for key, value := range m.value {
		tags := key.tags()
		if value.seen > 0 {
			_ = m.statsd.Count(MetricSamplerSeen, value.seen, tags, 1)
		}
		if value.kept > 0 {
			_ = m.statsd.Count(MetricSamplerKept, value.kept, tags, 1)
		}
	}
	m.value = make(map[MetricsKey]metricsValue) // reset counters
	m.valueMutex.Unlock()

	for _, mr := range m.additionalReporters {
		mr.report(m.statsd)
	}
}
