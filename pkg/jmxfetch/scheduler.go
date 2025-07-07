// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build jmx

package jmxfetch

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// JmxScheduler receives AD integration configs and updates JMXFetch configuration.
type JmxScheduler struct {
	configs map[string][]string
}

func newJmxScheduler() *JmxScheduler {
	return &JmxScheduler{
		configs: make(map[string][]string),
	}
}

// Schedule implements Scheduler#Schedule.
func (s *JmxScheduler) Schedule(configs []integration.Config) {
	for _, config := range configs {
		if !config.IsCheckConfig() || config.HasFilter(workloadfilter.MetricsFilter) {
			continue
		}

		digest := config.Digest()

		for _, instance := range config.Instances {
			if !check.IsJMXInstance(config.Name, instance, config.InitConfig) {
				continue
			}

			c := integration.Config{
				ADIdentifiers: config.ADIdentifiers,
				ServiceID:     config.ServiceID,
				InitConfig:    config.InitConfig,
				Instances:     []integration.Data{instance},
				LogsConfig:    config.LogsConfig,
				MetricConfig:  config.MetricConfig,
				Name:          config.Name,
				Provider:      config.Provider,
			}

			id := fmt.Sprintf("%v_%x", c.Name, c.IntDigest())
			log.Debugf("Scheduling jmxfetch config: %v: %q", id, c.String())

			if err := state.runner.configureRunner(instance, config.InitConfig); err != nil {
				log.Errorf("Could not configure jmxfetch for %v: %v", id, err)
				continue
			}

			s.configs[digest] = append(s.configs[digest], id)

			if err := state.scheduleConfig(id, c); err != nil {
				log.Errorf("Could not schedule jmxfetch config: %v: %v", id, err)
			}
		}
	}
}

// Unschedule removes check configurations from jmxfetch state.
func (s *JmxScheduler) Unschedule(configs []integration.Config) {
	for _, config := range configs {
		digest := config.Digest()
		for _, id := range s.configs[digest] {
			log.Debugf("Unschedling jmxfetch config: %v", id)
			state.unscheduleConfig(id)
		}
		delete(s.configs, digest)
	}
}

// Stop implements Scheduler#Stop.
func (s *JmxScheduler) Stop() {
}

// RegisterWith adds the JMX scheduler to receive events from the autodiscovery.
func RegisterWith(ac autodiscovery.Component) {
	ac.AddScheduler("jmx", newJmxScheduler(), true)
}
