// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package scheduler

import (
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
)

// MetaScheduler is a scheduler dispatching to all registered schedulers
type MetaScheduler struct{}

// Start inits the meta scheduler
func (s *MetaScheduler) Start() {
	// no op
}

// ScheduleConfigs schedules configs to all registered schedulers
func (s *MetaScheduler) ScheduleConfigs(configs []integration.Config) {
	for _, scheduler := range DefaultCatalog {
		scheduler.ScheduleConfigs(configs)
	}
}

// UnscheduleConfigs unschedules configs to all registered schedulers
func (s *MetaScheduler) UnscheduleConfigs(configs []integration.Config) {
	for _, scheduler := range DefaultCatalog {
		scheduler.UnscheduleConfigs(configs)
	}
}

// Stop handles clean stop of registered schedulers
func (s *MetaScheduler) Stop() {
	for _, scheduler := range DefaultCatalog {
		scheduler.Stop()
	}
}

// GetScheduler returns a registered scheduler
func (s *MetaScheduler) GetScheduler(name string) Scheduler {
	for key, scheduler := range DefaultCatalog {
		if name == key {
			return scheduler
		}
	}
	return nil
}
