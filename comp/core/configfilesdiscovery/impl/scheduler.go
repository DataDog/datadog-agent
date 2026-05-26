// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package configfilesdiscoveryimpl

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/scheduler"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const schedulerName = "configfiles-discovery"

type adScheduler struct {
	resolver targetResolver
}

var _ scheduler.Scheduler = (*adScheduler)(nil)

// newADScheduler builds the object registered with autodiscovery.
// Autodiscovery calls this scheduler when integration configs appear or
// disappear; this component only uses the scheduled configs as triggers for
// one-shot config collection.
func newADScheduler(resolver targetResolver) *adScheduler {
	return &adScheduler{
		resolver: resolver,
	}
}

// Schedule is called by autodiscovery with configs that should run for a
// service. For each config, it normalizes the AD service information into an
// internal target, selects the collector for the integration name, builds a
// runtime-specific reader for the target service, and runs the collector once.
func (s *adScheduler) Schedule(configs []integration.Config) {
	for _, config := range configs {
		target, ok := s.resolver.Resolve(config)
		if !ok {
			continue
		}

		collector, ok := configCollectors[config.Name]
		if !ok {
			log.Debugf("config files discovery has no collector for integration %q service %q", config.Name, config.ServiceID)
			continue
		}

		readerFactory, ok := configReaders[target.runtime]
		if !ok {
			log.Debugf("config files discovery has no config reader for integration %q service %q runtime %q", config.Name, config.ServiceID, target.runtime)
			continue
		}

		reader, err := readerFactory(target)
		if err != nil {
			log.Warnf("failed to build config reader for integration %q service %q runtime %q: %v", config.Name, config.ServiceID, target.runtime, err)
			continue
		}

		if err := collector.Run(context.Background(), reader); err != nil {
			log.Warnf("failed to run config files discovery for integration %q service %q: %v", config.Name, config.ServiceID, err)
			continue
		}
	}
}

// Unschedule is required by the autodiscovery scheduler interface. Config file
// discovery does not keep a long-running collection tied to a scheduled AD
// config, so there is nothing to tear down when AD unschedules it.
func (s *adScheduler) Unschedule(_ []integration.Config) {}

// Stop is required by the autodiscovery scheduler interface. The component
// unregisters this scheduler from autodiscovery during shutdown, and collectors
// are currently one-shot, so there is no scheduler-owned state to drain.
func (s *adScheduler) Stop() {}
