// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sampler

import (
	"sync"

	"github.com/DataDog/datadog-go/v5/statsd"
)

const (
	metricSamplerSeen = "datadog.trace_agent.sampler.seen"
	metricSamplerKept = "datadog.trace_agent.sampler.kept"
	metricSamplerSize = "datadog.trace_agent.sampler.size"
)

type metrics struct {
	statsd     statsd.ClientInterface
	tags       []string
	valueMutex sync.Mutex
	value      map[metricsKey]metricsValue
}

type metricsKey struct {
	targetService    string
	targetEnv        string
	samplingPriority string
}

func newMetricsKey(service, env string, samplingPriority *SamplingPriority) metricsKey {
	mk := metricsKey{
		targetService: service,
		targetEnv:     env,
	}
	if samplingPriority != nil {
		mk.samplingPriority = samplingPriority.tagValue()
	}
	return mk
}

func (k metricsKey) tags() []string {
	tags := make([]string, 0, 3) // Pre-allocate number of fields for efficiency
	if k.targetService != "" {
		tags = append(tags, "target_service:"+k.targetService)
	}
	if k.targetEnv != "" {
		tags = append(tags, "target_env:"+k.targetEnv)
	}
	if k.samplingPriority != "" {
		tags = append(tags, "sampling_priority:"+k.samplingPriority)
	}
	return tags
}

type metricsValue struct {
	seen int64
	kept int64
}

func (m *metrics) record(sampled bool, metricsKey metricsKey) {
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

func (m *metrics) report() {
	m.valueMutex.Lock()
	defer m.valueMutex.Unlock()
	for key, value := range m.value {
		tags := append(m.tags, key.tags()...)
		if value.seen > 0 {
			_ = m.statsd.Count(metricSamplerSeen, value.seen, tags, 1)
		}
		if value.kept > 0 {
			_ = m.statsd.Count(metricSamplerKept, value.kept, tags, 1)
		}
	}
	m.value = make(map[metricsKey]metricsValue) // reset counters
}
