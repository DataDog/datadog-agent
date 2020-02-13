// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package scheduler

import (
	"fmt"
	"strings"

	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers/names"
	logsConfig "github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Scheduler creates and deletes new sources and services to start or stop
// log collection on different kind of inputs.
// A source represents a logs-config that can be defined either in a configuration file,
// in a docker label or a pod annotation.
// A service represents a process that is actually running on the host like a container for example.
type Scheduler struct {
	sources  *logsConfig.LogSources
	services *service.Services
}

// NewScheduler returns a new scheduler.
func NewScheduler(sources *logsConfig.LogSources, services *service.Services) *Scheduler {
	return &Scheduler{
		sources:  sources,
		services: services,
	}
}

// Stop does nothing.
func (s *Scheduler) Stop() {}

// Schedule creates new sources and services from a list of integration configs.
// An integration config can be mapped to a list of sources when it contains a Provider,
// while an integration config can be mapped to a service when it contains an Entity.
// An entity represents a unique identifier for a process that be reused to query logs.
func (s *Scheduler) Schedule(configs []integration.Config) {
	for _, config := range configs {
		if !config.IsLogConfig() {
			continue
		}
		switch {
		case s.newSources(config):
			log.Infof("Received a new logs config: %v", s.configName(config))
			sources, err := s.toSources(config)
			if err != nil {
				log.Warnf("Invalid configuration: %v", err)
				continue
			}
			for _, source := range sources {
				s.sources.AddSource(source)
			}
		case s.newService(config):
			entityType, _, err := s.parseEntity(config.TaggerEntity)
			if err != nil {
				log.Warnf("Invalid service: %v", err)
				continue
			}
			// logs only consider container services
			if entityType != containers.ContainerEntityName {
				continue
			}
			log.Infof("Received a new service: %v", config.Entity)
			service, err := s.toService(config)
			if err != nil {
				log.Warnf("Invalid service: %v", err)
				continue
			}
			s.services.AddService(service)
		default:
			// invalid integration config
			continue
		}
	}
}

// Unschedule removes all the sources and services matching the integration configs.
func (s *Scheduler) Unschedule(configs []integration.Config) {
	for _, config := range configs {
		if !config.IsLogConfig() {
			continue
		}
		switch {
		case s.newSources(config):
			log.Infof("New source to remove: entity: %v", config.Entity)

			_, identifier, err := s.parseEntity(config.Entity)
			if err != nil {
				log.Warnf("Invalid configuration: %v", err)
				continue
			}

			for _, source := range s.sources.GetSources() {
				if identifier == source.Config.Identifier {
					s.sources.RemoveSource(source)
				}
			}
		case s.newService(config):
			// new service to remove
			entityType, _, err := s.parseEntity(config.TaggerEntity)
			if err != nil {
				log.Warnf("Invalid service: %v", err)
				continue
			}
			// logs only consider container services
			if entityType != containers.ContainerEntityName {
				continue
			}
			log.Infof("New service to remove: entity: %v", config.Entity)
			service, err := s.toService(config)
			if err != nil {
				log.Warnf("Invalid service: %v", err)
				continue
			}
			s.services.RemoveService(service)
		default:
			// invalid integration config
			continue
		}
	}
}

// newSources returns true if the config can be mapped to sources.
func (s *Scheduler) newSources(config integration.Config) bool {
	return config.Provider != ""
}

// newService returns true if the config can be mapped to a service.
func (s *Scheduler) newService(config integration.Config) bool {
	return config.Provider == "" && config.Entity != ""
}

// configName returns the name of the configuration.
func (s *Scheduler) configName(config integration.Config) string {
	if config.Name != "" {
		return config.Name
	}
	service, err := s.toService(config)
	if err == nil {
		return service.Type
	}
	return config.Provider
}

// toSources creates new sources from an integration config,
// returns an error if the parsing failed.
func (s *Scheduler) toSources(config integration.Config) ([]*logsConfig.LogSource, error) {
	var configs []*logsConfig.LogsConfig
	var err error

	switch config.Provider {
	case names.File:
		// config defined in a file
		configs, err = logsConfig.ParseYAML(config.LogsConfig)
	case names.Docker, names.Kubernetes:
		// config attached to a docker label or a pod annotation
		configs, err = logsConfig.ParseJSON(config.LogsConfig)
	default:
		// invalid provider
		err = fmt.Errorf("parsing logs config from %v is not supported yet", config.Provider)
	}
	if err != nil {
		return nil, err
	}

	var service *service.Service

	commonGlobalOptions := integration.CommonGlobalConfig{}
	err = yaml.Unmarshal(config.InitConfig, &commonGlobalOptions)
	if err != nil {
		return nil, fmt.Errorf("invalid init_config section for source %s: %s", config.Name, err)
	}

	globalServiceDefined := len(commonGlobalOptions.Service) > 0

	if config.Entity != "" {
		// all configs attached to a docker label or a pod annotation contains an entity;
		// this entity is used later on by an input to match a service with a source
		// to start collecting logs.
		var err error
		service, err = s.toService(config)
		if err != nil {
			return nil, fmt.Errorf("invalid entity: %v", err)
		}
	}

	configName := s.configName(config)
	var sources []*logsConfig.LogSource
	for _, cfg := range configs {
		// if no service is set fall back to the global one
		if len(cfg.Service) == 0 && globalServiceDefined {
			cfg.Service = commonGlobalOptions.Service
		}

		if service != nil {
			// a config defined in a docker label or a pod annotation does not always contain a type,
			// override it here to ensure that the config won't be dropped at validation.
			cfg.Type = service.Type
			cfg.Identifier = service.Identifier // used for matching a source with a service
		}

		source := logsConfig.NewLogSource(configName, cfg)
		sources = append(sources, source)
		if err := cfg.Validate(); err != nil {
			log.Warnf("Invalid logs configuration: %v", err)
			source.Status.Error(err)
			continue
		}
	}

	return sources, nil
}

// toService creates a new service for an integrationConfig.
func (s *Scheduler) toService(config integration.Config) (*service.Service, error) {
	provider, identifier, err := s.parseEntity(config.Entity)
	if err != nil {
		return nil, err
	}
	return service.NewService(provider, identifier, s.getCreationTime(config)), nil
}

// parseEntity breaks down an entity into a service provider and a service identifier.
func (s *Scheduler) parseEntity(entity string) (string, string, error) {
	components := strings.Split(entity, containers.EntitySeparator)
	if len(components) != 2 {
		return "", "", fmt.Errorf("entity is malformed : %v", entity)
	}
	return components[0], components[1], nil
}

// integrationToServiceCRTime maps an integration creation time to a service creation time.
var integrationToServiceCRTime = map[integration.CreationTime]service.CreationTime{
	integration.Before: service.Before,
	integration.After:  service.After,
}

// getCreationTime returns the service creation time for the integration configuration.
func (s *Scheduler) getCreationTime(config integration.Config) service.CreationTime {
	return integrationToServiceCRTime[config.CreationTime]
}
