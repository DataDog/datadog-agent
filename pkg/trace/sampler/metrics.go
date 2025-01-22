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
	statsd statsd.ClientInterface
	tags   []string
	value  sync.Map
}

type metricsKey [3]string

func newMetricsKey(service, env string, samplingPriority *SamplingPriority) metricsKey {
	var key metricsKey
	if service != "" {
		key[0] = "target_service:" + service
	}
	if env != "" {
		key[1] = "target_env:" + env
	}
	if samplingPriority != nil {
		key[2] = samplingPriority.tag()
	}
	return key
}

func (k metricsKey) tags() []string {
	tags := make([]string, 0, len(k))
	for _, v := range k {
		if v != "" {
			tags = append(tags, v)
		}
	}
	return tags
}

type metricsValue struct {
	seen int64
	kept int64
}

func (m *metrics) record(sampled bool, metricsKey metricsKey) {
	initialValue := metricsValue{seen: 1}
	if sampled {
		initialValue.kept = 1
	}
	if v, load := m.value.LoadOrStore(metricsKey, initialValue); load {
		loadedMetricsValue := v.(metricsValue)
		loadedMetricsValue.seen++
		if sampled {
			loadedMetricsValue.kept++
		}
		m.value.Store(metricsKey, loadedMetricsValue)
	}
}

func (m *metrics) report() {
	m.value.Range(func(key, value any) bool {
		metricsKey := key.(metricsKey)
		metricsValue := value.(metricsValue)
		tags := append(m.tags, metricsKey.tags()...)
		if metricsValue.seen > 0 {
			_ = m.statsd.Count(metricSamplerSeen, metricsValue.seen, tags, 1)
		}
		if metricsValue.kept > 0 {
			_ = m.statsd.Count(metricSamplerKept, metricsValue.kept, tags, 1)
		}
		m.value.Delete(metricsKey) // reset counters
		return true
	})
}
