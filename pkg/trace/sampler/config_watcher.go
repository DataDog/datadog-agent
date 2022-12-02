// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sampler

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"time"
)

type ConfigWatcher struct {
	prioritySampler *PrioritySampler
	errorsSampler   *ErrorsSampler
	rareSampler     *RareSampler
	stop            chan struct{}
}

func NewConfigWatcher(prioritySampler *PrioritySampler, errorsSampler *ErrorsSampler, rareSampler *RareSampler) *ConfigWatcher {
	return &ConfigWatcher{
		prioritySampler: prioritySampler,
		errorsSampler:   errorsSampler,
		rareSampler:     rareSampler,
		stop:            make(chan struct{}),
	}
}

func (w *ConfigWatcher) Start() {
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				metrics.Gauge("datadog.trace_agent.sampler.priority_sampler_target_tps", w.prioritySampler.sampler.GetTargetTps(), []string{fmt.Sprintf("remotely_configured:%v", w.prioritySampler.RemotelyConfigured)}, 1)
				metrics.Gauge("datadog.trace_agent.sampler.errors_sampler_target_tps", w.errorsSampler.GetTargetTps(), []string{fmt.Sprintf("remotely_configured:%v", w.errorsSampler.RemotelyConfigured)}, 1)
				metrics.Gauge("datadog.trace_agent.sampler.rare_sampler_target_tps", float64(w.rareSampler.limiter.Limit()), []string{fmt.Sprintf("enabled:%v", w.rareSampler.GetEnabled()), fmt.Sprintf("remotely_configured:%v", w.rareSampler.RemotelyConfigured)}, 1)
			case <-w.stop:
				return
			}
		}
	}()
}

func (w *ConfigWatcher) Stop() {
	close(w.stop)
}
