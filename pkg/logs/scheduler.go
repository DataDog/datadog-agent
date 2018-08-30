// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package logs

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
)

// Scheduler registers to autodiscovery to schedule/unschedule log-collection.
type Scheduler struct {
	sources  *config.LogSources
	services *service.Services
}

// NewScheduler returns a new scheduler.
func NewScheduler(sources *config.LogSources, services *service.Services) *Scheduler {
	return &Scheduler{
		sources:  sources,
		services: services,
	}
}

// Stop does nothing.
func (s *Scheduler) Stop() {}

// Schedule creates new log-sources from configs.
func (s *Scheduler) Schedule(configs []integration.Config) {
	for _, config := range configs {
		if !s.isLogConfig(config) {
			continue
		}
		if config.Provider != "" {
			log.Infof("Received new logs config for integration: %v", config.Name)
			sources, err := s.toSources(config)
			if err != nil {
				log.Warnf("Invalid configuration: %v", err)
				continue
			}
			for _, source := range sources {
				s.sources.AddSource(source)
			}
		} else {
			log.Infof("Received a new service with entity: %v", config.Entity)
			service, err := s.toService(config)
			if err != nil {
				log.Warnf("Invalid service: %v", err)
				continue
			}
			s.services.AddService(service)
		}
	}
}

// Unschedule removes services that have been stopped.
func (s *Scheduler) Unschedule(configs []integration.Config) {
	for _, config := range configs {
		if !s.isLogConfig(config) {
			continue
		}
		if config.Provider != "" {
			continue
		}
		log.Infof("New service to remove with entity: %v", config.Entity)
		service, err := s.toService(config)
		if err != nil {
			log.Warnf("Invalid service: %v", err)
			continue
		}
		s.services.RemoveService(service)
	}
}

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
		configs, err = config.ParseYAML(integrationConfig.LogsConfig)
	case providers.Docker, providers.Kubernetes:
		configs, err = config.ParseJSON(integrationConfig.LogsConfig)
	default:
		err = fmt.Errorf("parsing logs config from %v is not supported yet", integrationConfig.Provider)
	}
	if err != nil {
		return nil, err
	}

	var sources []*config.LogSource
	for _, cfg := range configs {
		integrationName := integrationConfig.Name
		if integrationConfig.Entity != "" {
			components := strings.Split(integrationConfig.Entity, "://")
			if len(components) != 2 {
				return nil, fmt.Errorf("entity is malformed : %v", integrationConfig.Entity)
			}
			service, identifier := components[0], components[1]
			integrationName = service
			cfg.Type = service
			cfg.Identifier = identifier
		}
		source := config.NewLogSource(integrationName, cfg)
		sources = append(sources, source)
		if err := cfg.Validate(); err != nil {
			log.Warnf("Invalid logs configuration: %v", err)
			source.Status.Error(err)
			continue
		}
		if err := cfg.Compile(); err != nil {
			log.Warnf("Could not compile logs configuration: %v", err)
			source.Status.Error(err)
			continue
		}
	}

	return sources, nil
}

// toService creates a new service for an integrationConfig.
func (s *Scheduler) toService(integrationConfig integration.Config) (*service.Service, error) {
	components := strings.Split(integrationConfig.Entity, "://")
	if len(components) != 2 {
		return nil, fmt.Errorf("entity is malformed : %v", integrationConfig.Entity)
	}
	provider, identifier := components[0], components[1]
	switch provider {
	case service.Docker:
		var crTime service.CreationTime
		switch integrationConfig.CreationTime {
		case integration.Before:
			crTime = service.Before
		case integration.After:
			crTime = service.After
		}
		return service.NewService(provider, identifier, crTime), nil
	default:
		return nil, fmt.Errorf("%v is not supported yet", provider)
	}
}
