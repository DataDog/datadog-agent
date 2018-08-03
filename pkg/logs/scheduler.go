// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package logs

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
)

// Scheduler registers to autodiscovery to schedule/unschedule log-collection.
type Scheduler struct {
	sources *config.LogSources
}

// NewScheduler returns a new scheduler.
func NewScheduler(sources *config.LogSources) *Scheduler {
	return &Scheduler{
		sources: sources,
	}
}

// Start does nothing.
func (s *Scheduler) Start() {}

// Stop does nothing.
func (s *Scheduler) Stop() {}

// Schedule creates new log-sources from configs.
func (s *Scheduler) Schedule(configs []integration.Config) {
	for _, config := range configs {
		if !s.isLogConfig(config) {
			continue
		}
		log.Infof("Received new logs config for integration: %v", config.Name)
		sources, err := s.toSources(config)
		if err != nil {
			log.Warnf("Invalid configuration: %v", err)
			continue
		}
		for _, source := range sources {
			s.sources.AddSource(source)
		}
	}
}

// Unschedule does nothing.
func (s *Scheduler) Unschedule(configs []integration.Config) {}

// isLogConfig returns true if config contains a logs config.
func (s *Scheduler) isLogConfig(config integration.Config) bool {
	return config.LogsConfig != nil
}

// toSources creates new logs-sources from an integration config,
// returns an error if the parsing failed.
func (s *Scheduler) toSources(integrationConfig integration.Config) ([]*config.LogSource, error) {
	var configs []*config.LogsConfig
	var err error

	switch integrationConfig.Provider {
	case providers.File:
		configs, err = config.ParseYaml(integrationConfig.LogsConfig)
	case providers.Docker, providers.Kubernetes:
		configs, err = config.ParseJSON(integrationConfig.LogsConfig)
	default:
		err = fmt.Errorf("parsing logs config from %v is not supported yet", integrationConfig.Provider)
	}
	if err != nil {
		return nil, err
	}

	var validConfigs []*config.LogsConfig
	for _, cfg := range configs {
		if err := config.Validate(cfg); err != nil {
			log.Warnf("Invalid logs configuration: %v", err)
			continue
		}
		if err := config.Compile(cfg); err != nil {
			log.Warnf("Could not compile logs configuration: %v", err)
			continue
		}
		validConfigs = append(validConfigs, cfg)
	}

	var sources []*config.LogSource
	for _, cfg := range configs {
		sources = append(sources, config.NewLogSource(integrationConfig.Name, cfg))
	}

	return sources, nil
}
