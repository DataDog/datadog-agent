// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package logs

import (
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
)

// Scheduler registers to autodiscovery to schedule/unschedule log-collection.
type Scheduler struct {
}

// NewScheduler returns a new scheduler.
func NewScheduler() *Scheduler {
	return &Scheduler{}
}

// Start does nothing
func (s *Scheduler) Start() {}

// Stop does nothing
func (s *Scheduler) Stop() {}

// Schedule creates new log-sources from configs.
func (s *Scheduler) Schedule(configs []integration.Config) {
	for _, config := range configs {
		if !s.isLogConfig(config) {
			continue
		}
		log.Infof("Received new logs-config for integration: %v", config.Name)
		sources, err := s.toSources(config)
		if err != nil {
			log.Warnf("Invalid configuration: %v", err)
			continue
		}
		for _, source := range sources {
			log.Infof("Adding source: %v", source)
		}
	}
}

// Unschedule invalidates all the log-sources coresponding to the configs.
func (s *Scheduler) Unschedule(configs []integration.Config) {
	for _, config := range configs {
		if !s.isLogConfig(config) {
			continue
		}
		log.Infof("Must invalidate logs-config for integration: %v", config.Name)
	}
}

// isLogConfig returns true if config contains a logs config.
func (s *Scheduler) isLogConfig(config integration.Config) bool {
	return config.LogsConfig != nil
}

// toSources creates a new logs-source,
// if the parsing failed, returns an error.
func (s *Scheduler) toSources(integrationConfig integration.Config) ([]*config.LogSource, error) {
	configs, err := config.Parse(logsConfigString)
	if err != nil {
		return nil, err
	}
	var sources []*config.LogSource
	for _, cfg := range configs {
		sources = append(sources, config.NewLogSource(integrationConfig.Name, cfg))
	}
	return sources, nil
}

// toSources creates a new logs-source,
// if the parsing failed, returns an error.
func (s *Scheduler) parse(cfg integration.Config) ([]*config.LogsConfig, error) {
	var configs []*config.LogsConfig
	switch config.Provider {
	case "":
	default:
		break
	}
	// configs, err := config.Parse(logsConfigString)
	// if err != nil {
	// 	return nil, err
	// }
	// var sources []*config.LogSource
	// for _, cfg := range configs {
	// 	sources = append(sources, config.NewLogSource(integrationConfig.Name, cfg))
	// }
	// return sources, nil
}
