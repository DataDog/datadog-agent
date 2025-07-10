// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package ad

import (
	"fmt"
	"strings"

	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	logsConfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/util/adlistener"
	"github.com/DataDog/datadog-agent/pkg/logs/schedulers"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
	sourcesPkg "github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Scheduler creates and deletes new sources and services to start or stop
// log collection based on information from autodiscovery.
//
// This type implements  pkg/logs/schedulers.Scheduler.
type Scheduler struct {
	mgr      schedulers.SourceManager
	listener *adlistener.ADListener
}

var _ schedulers.Scheduler = &Scheduler{}

// New creates a new scheduler.
func New(ac autodiscovery.Component) schedulers.Scheduler {
	sch := &Scheduler{}
	sch.listener = adlistener.NewADListener("logs-agent AD scheduler", ac, sch.Schedule, sch.Unschedule)
	return sch
}

// Start implements schedulers.Scheduler#Start.
func (s *Scheduler) Start(sourceMgr schedulers.SourceManager) {
	s.mgr = sourceMgr
	s.listener.StartListener()
}

// Stop implements schedulers.Scheduler#Stop.
func (s *Scheduler) Stop() {
	s.listener.StopListener()
	s.mgr = nil
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
		if config.HasFilter(workloadfilter.LogsFilter) {
			log.Debugf("Config %s is filtered out for logs collection, ignoring it", configName(config))
			continue
		}
		switch {
		case s.newSources(config):
			log.Infof("Received a new logs config: %v", configName(config))
			sources, err := CreateSources(config)
			if err != nil {
				log.Warnf("Invalid configuration: %v", err)
				continue
			}
			for _, source := range sources {
				s.mgr.AddSource(source)
			}
		default:
			log.Debugf("Invalid integration config: %s, ignoring it", configName(config))
			continue
		}
	}
}

// Unschedule removes all the sources and services matching the integration configs.
func (s *Scheduler) Unschedule(configs []integration.Config) {
	for _, config := range configs {
		if !config.IsLogConfig() || config.HasFilter(workloadfilter.LogsFilter) {
			continue
		}
		switch {
		case s.newSources(config):
			log.Infof("New source to remove: entity: %v", config.ServiceID)

			_, identifier, err := parseServiceID(config.ServiceID)
			if err != nil {
				log.Warnf("Invalid configuration: %v", err)
				continue
			}

			// remove all the sources for this ServiceID.  This makes the
			// implicit, and not-quite-correct assumption that we only ever
			// receive one config for a given ServiceID, and that it generates
			// the same sources.
			//
			// This may also remove sources not added by this scheduler, for
			// example sources added by other schedulers or sources added by
			// launchers.
			for _, source := range s.mgr.GetSources() {
				if identifier == source.Config.Identifier {
					s.mgr.RemoveSource(source)
				}
			}
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

// configName returns the name of the configuration.
func configName(config integration.Config) string {
	if config.Name != "" {
		return config.Name
	}
	service, err := toService(config)
	if err == nil {
		return service.Type
	}
	return config.Provider
}

// createsSources creates new sources from an integration config,
// returns an error if the parsing failed.
func CreateSources(config integration.Config) ([]*sourcesPkg.LogSource, error) {
	var configs []*logsConfig.LogsConfig
	var err error

	switch config.Provider {
	case names.File:
		// config defined in a file
		configs, err = logsConfig.ParseYAML(config.LogsConfig)
	case names.Container, names.Kubernetes, names.KubeContainer:
		// config attached to a container label or a pod annotation
		configs, err = logsConfig.ParseJSON(config.LogsConfig)
	case names.RemoteConfig:
		if pkgconfigsetup.Datadog().GetBool("remote_configuration.agent_integrations.allow_log_config_scheduling") {
			// config supplied by remote config
			configs, err = logsConfig.ParseJSON(config.LogsConfig)
		} else {
			log.Warnf("parsing logs config from %v is disabled. You can enable it by setting remote_configuration.agent_integrations.allow_log_config_scheduling to true", names.RemoteConfig)
		}
	case names.DataStreamsLiveMessages:
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

	if config.ServiceID != "" {
		// all configs attached to a docker label or a pod annotation contains an entity;
		// this entity is used later on by an input to match a service with a source
		// to start collecting logs.
		var err error
		service, err = toService(config)
		if err != nil {
			return nil, fmt.Errorf("invalid entity: %v", err)
		}
	}

	configName := configName(config)
	var sources []*sourcesPkg.LogSource
	for _, cfg := range configs {
		// if no service is set fall back to the global one
		if cfg.Service == "" && globalServiceDefined {
			cfg.Service = commonGlobalOptions.Service
		}

		if service != nil {
			// a config defined in a container label or a pod annotation does not always contain a type,
			// override it here to ensure that the config won't be dropped at validation.
			if (cfg.Type == logsConfig.FileType || cfg.Type == logsConfig.TCPType || cfg.Type == logsConfig.UDPType) && (config.Provider == names.Kubernetes || config.Provider == names.Container || config.Provider == names.KubeContainer || config.Provider == logsConfig.FileType) {
				// cfg.Type is not overwritten as tailing a file from a Docker or Kubernetes AD configuration
				// is explicitly supported (other combinations may be supported later)
				cfg.Identifier = service.Identifier
			} else {
				cfg.Type = service.Type
				cfg.Identifier = service.Identifier // used for matching a source with a service
			}
		}

		source := sourcesPkg.NewLogSource(configName, cfg)
		if source.Config.IntegrationName == "" {
			// If the log integration comes from a config file, we try to match it with the config name
			// that is most likely the integration name.
			// If it comes from a container environment, the name was computed based on the `check_names`
			// labels attached to the same container.
			source.Config.IntegrationName = configName
		}
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
func toService(config integration.Config) (*service.Service, error) {
	provider, identifier, err := parseServiceID(config.ServiceID)
	if err != nil {
		return nil, err
	}
	return service.NewService(provider, identifier), nil
}

// parseServiceID breaks down an AD service ID, assuming it is formatted
// as `something://something-else`, into its consituent parts.
func parseServiceID(serviceID string) (string, string, error) {
	components := strings.Split(serviceID, containers.EntitySeparator)
	if len(components) != 2 {
		return "", "", fmt.Errorf("service ID does not have the form `xxx://yyy`: %v", serviceID)
	}
	return components[0], components[1], nil
}
