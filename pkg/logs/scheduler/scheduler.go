// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scheduler

import (
	"context"
	"fmt"
	"strings"

	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	logsConfig "github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

// The logs scheduler is connected to autodiscovery via the agent's
// common.LoadComponents method, which calls its Schedule and Unschedule
// methods.  For containers, the scheduler uses workloadmeta, rather than
// autodiscovery.

var (
	// scheduler is plugged to autodiscovery to collect integration configs
	// and schedule log collection for different kind of inputs
	adScheduler *Scheduler
)

// Scheduler creates and deletes new sources and services to start or stop
// log collection on different kind of inputs.
// A source represents a logs-config that can be defined either in a configuration file,
// in a docker label or a pod annotation.
// A service represents a process that is actually running on the host like a container for example.
type Scheduler struct {
	sources  *logsConfig.LogSources
	services *service.Services

	// context controls the lifetime of this service
	ctx context.Context

	// cancel cancels the context
	cancel context.CancelFunc

	// events from the workloadmeta store
	events chan workloadmeta.EventBundle

	// services we have added, so that we can remove them again, indexed by ID
	addedServices map[workloadmeta.EntityID]*service.Service
}

// NewScheduler creates a scheduler that will report to the given LogSources
// and Services instances.  The scheduler is automatically started.
func NewScheduler(sources *logsConfig.LogSources, services *service.Services, wlms workloadmeta.Store) *Scheduler {
	filter := workloadmeta.NewFilter([]workloadmeta.Kind{workloadmeta.KindContainer}, nil)
	events := wlms.Subscribe("logs", filter)

	ctx, cancel := context.WithCancel(context.Background())
	sch := &Scheduler{
		sources:       sources,
		services:      services,
		ctx:           ctx,
		cancel:        cancel,
		events:        events,
		addedServices: make(map[workloadmeta.EntityID]*service.Service),
	}
	sch.start()
	return sch
}

// CreateScheduler creates the global scheduler (which will subsequently be
// returned from GetScheduler)
func CreateScheduler(sources *logsConfig.LogSources, services *service.Services, wlms workloadmeta.Store) {
	adScheduler = NewScheduler(sources, services, wlms)
}

// start begins listening for scheduling events
func (s *Scheduler) start() {
	go func() {
		// logs services have a "creationTime" relative to agent startup: Before or
		// After.  This is used to determine whether we are logging something that
		// has already been running (Before) or something new (After), and thus
		// whether to start at the end (Before) or beginning (After) of the
		// logfile.  Workloadmeta does not give us this information directly, but
		// it does produce an EventBundle at subscription time containing all of
		// the already-detected services.  So, we treat this first bundle as Before
		// and all subsequent as After.
		creationTime := service.Before

		for {
			select {
			case <-s.ctx.Done():
				return
			case evtbundle, open := <-s.events:
				if !open {
					return
				}
				for _, e := range evtbundle.Events {
					switch e.Type {
					case workloadmeta.EventTypeSet:
						s.handleWorkloadMetaSetEvent(e, creationTime)
					case workloadmeta.EventTypeUnset:
						s.handleWorkloadMetaUnsetEvent(e)
					}
				}
				close(evtbundle.Ch)
				creationTime = service.After
			}
		}
	}()
}

// Stop stops the scheduler.
func (s *Scheduler) Stop() {
	// cancel the context, signalling the listening goroutine to stop
	s.cancel()

	// unsubscribe from workloadmeta events
	wlms := workloadmeta.GetGlobalStore()
	wlms.Unsubscribe(s.events)

	// if this was the global scheduler, remove it
	if s == adScheduler {
		adScheduler = nil
	}
}

// Schedule creates new sources and services from a list of integration configs.
// An integration config can be mapped to a list of sources when it contains a Provider,
// while an integration config can be mapped to a service when it contains an Entity.
// An entity represents a unique identifier for a process that be reused to query logs.
func (s *Scheduler) Schedule(configs []integration.Config) {
	for _, config := range configs {
		if !config.IsLogConfig() {
			continue
		}
		if config.HasFilter(containers.LogsFilter) {
			log.Debugf("Config %s is filtered out for logs collection, ignoring it", s.configName(config))
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
		default:
			// invalid integration config
			continue
		}
	}
}

// Unschedule removes all the sources and services matching the integration configs.
func (s *Scheduler) Unschedule(configs []integration.Config) {
	for _, config := range configs {
		if !config.IsLogConfig() || config.HasFilter(containers.LogsFilter) {
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
		default:
			// invalid integration config
			continue
		}
	}
}

// handleWorkloadMetaSetEvent handles an incoming event from the workload
// metadata service indicating that an entity has been "set".  The creationTime
// argument is an estimate of whether this workload started Before or After the
// agent started.
func (s *Scheduler) handleWorkloadMetaSetEvent(e workloadmeta.Event, creationTime service.CreationTime) {
	container := e.Entity.(*workloadmeta.Container)

	var svc *service.Service
	switch container.Runtime {
	case workloadmeta.ContainerRuntimeDocker:
		if _, exists := s.addedServices[e.Entity.GetID()]; exists {
			// container already has a service
			return
		}
		svc = service.NewService(config.DockerType, container.ID, creationTime)
	default:
		// unknown runtime, so no logging
		return
	}
	s.addedServices[e.Entity.GetID()] = svc
	s.services.AddService(svc)
}

// handleWorkloadMetaUnsetEvent handles an incoming event from the workload metadata
// service indicating that an entity has been "unset"
func (s *Scheduler) handleWorkloadMetaUnsetEvent(e workloadmeta.Event) {
	// We don't get a workloadmeta.Container in e.Entity, so we do not know the
	// container runtime for this container and must find the container we have
	// previously added to s.services.
	id := e.Entity.GetID()
	svc, found := s.addedServices[id]
	if !found {
		log.Debugf("Container %s was not being logged; nothing to do to stop logging", id)
		return
	}
	s.services.RemoveService(svc)
	delete(s.addedServices, id)
}

// newSources returns true if the config can be mapped to sources.
func (s *Scheduler) newSources(config integration.Config) bool {
	return config.Provider != ""
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
	case names.Container, names.Kubernetes:
		// config attached to a container label or a pod annotation
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

	globalServiceDefined := commonGlobalOptions.Service != ""

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
		if cfg.Service == "" && globalServiceDefined {
			cfg.Service = commonGlobalOptions.Service
		}

		if service != nil {
			// a config defined in a container label or a pod annotation does not always contain a type,
			// override it here to ensure that the config won't be dropped at validation.
			if cfg.Type == logsConfig.FileType && (config.Provider == names.Kubernetes || config.Provider == names.Container) {
				// cfg.Type is not overwritten as tailing a file from a Docker or Kubernetes AD configuration
				// is explicitly supported (other combinations may be supported later)
				cfg.Identifier = service.Identifier
			} else {
				cfg.Type = service.Type
				cfg.Identifier = service.Identifier // used for matching a source with a service
			}
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

// GetScheduler returns the logs-config scheduler if set.
func GetScheduler() *Scheduler {
	return adScheduler
}

// GetSourceFromName returns the LogSource from the source name if it exists.
func (s *Scheduler) GetSourceFromName(name string) *logsConfig.LogSource {
	for _, source := range s.sources.GetSources() {
		if name == source.Config.Source {
			return source
		}
	}
	return nil
}
