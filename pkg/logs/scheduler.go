// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package logs

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
)

// Scheduler registers to autodiscovery to schedule/unschedule log-collection.
type Scheduler struct {
	// activeSources map[string]*config.LogSource
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
		// sources, err := s.toSources(config)
		// if err != nil {
		// 	log.Warnf("Invalid configuration: %v", err)
		// 	continue
		// }
		// configId, err := s.getConfigIdentifier(config)
		// if err != nil {
		// 	log.Warnf("Invalid configuration: %v", err)
		// 	continue
		// }
		// for _, source := range sources {
		// 	log.Infof("Adding source with id: %v", configId)
		// 	s.activeSources[configId] = source
		// }
	}
}

// Unschedule invalidates all the log-sources coresponding to the configs.
func (s *Scheduler) Unschedule(configs []integration.Config) {
	for _, config := range configs {
		if !s.isLogConfig(config) {
			continue
		}
		log.Infof("Must invalidate logs-config for integration: %v", config.Name)
		// configId, err := s.getConfigIdentifier(config)
		// if err != nil {
		// 	continue
		// }
		// source, exists := s.activeSources[configId]
		// if !exists {
		// 	// this config has not been processed in the past.
		// 	continue
		// }
		// log.Infof("Removing source with id: %v", configId)
		// delete(s.activeSources, configId)
	}
}

// isLogConfig returns true if config contains a logs config.
func (s *Scheduler) isLogConfig(config integration.Config) bool {
	return config.LogsConfig != nil
}

// toSources creates a new logs-source,
// if the parsing failed, returns an error.
func (s *Scheduler) toSources(integrationConfig integration.Config) ([]*config.LogSource, error) {
	logsConfigString := string(integrationConfig.LogsConfig)
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

// getConfigIdentifier returns the unique identifier of the configuration.
func (s *Scheduler) getConfigIdentifier(config integration.Config) (string, error) {
	identifiers := config.ADIdentifiers
	if len(identifiers) < 1 {
		// this should never occur
		return "", fmt.Errorf("no identifiers provided in config: %v", config.Name)
	}
	return identifiers[0], nil
}
