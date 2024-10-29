// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package listeners

import (
	"context"
	"errors"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// ContainerPort represents a network port in a Service.
type ContainerPort struct {
	Port int
	Name string
}

// Service represents an application we can run a check against.
// It should be matched with a check template by the ConfigResolver using the
// ADIdentifiers field.
type Service interface {
	Equal(Service) bool                                          // compare two services
	GetServiceID() string                                        // unique service name
	GetADIdentifiers(context.Context) ([]string, error)          // identifiers on which templates will be matched
	GetHosts(context.Context) (map[string]string, error)         // network --> IP address
	GetPorts(context.Context) ([]ContainerPort, error)           // network ports
	GetTags() ([]string, error)                                  // tags
	GetTagsWithCardinality(cardinality string) ([]string, error) // tags with given cardinality
	GetPid(context.Context) (int, error)                         // process identifier
	GetHostname(context.Context) (string, error)                 // hostname.domainname for the entity
	IsReady(context.Context) bool                                // is the service ready
	HasFilter(containers.FilterType) bool                        // whether the service is excluded by metrics or logs exclusion config
	GetExtraConfig(string) (string, error)                       // Extra configuration values

	// FilterTemplates filters the templates which will be resolved against
	// this service, in a map keyed by template digest.
	//
	// This method is called every time the configs for the service change,
	// with the full set of templates matching this service.  It must not rely
	// on any non-static information except the given configs, and it must not
	// modify the configs in the map.
	FilterTemplates(map[string]integration.Config)
}

// ServiceListener monitors running services and triggers check (un)scheduling
//
// It holds a cache of running services, listens to new/killed services and
// updates its cache, and the autoconfig with these events.
type ServiceListener interface {
	Listen(newSvc, delSvc chan<- Service)
	Stop()
}

// Config represents autodiscovery listener config
type Config interface {
	IsProviderEnabled(string) bool
}

// ServiceListernerDeps are the service listerner dependencies
type ServiceListernerDeps struct {
	Config    Config
	Telemetry *telemetry.Store
	Tagger    tagger.Component
	Wmeta     optional.Option[workloadmeta.Component]
}

// ServiceListenerFactory builds a service listener
type ServiceListenerFactory func(ServiceListernerDeps) (ServiceListener, error)

// Register registers a service listener factory
func Register(name string,
	factory ServiceListenerFactory,
	serviceListenerFactories map[string]ServiceListenerFactory) {
	if factory == nil {
		log.Infof("Service listener factory %s does not exist.", name)
		return
	}
	_, registered := serviceListenerFactories[name]
	if registered {
		log.Errorf("Service listener factory %s already registered. Ignoring.", name)
		return
	}
	serviceListenerFactories[name] = factory
}

// ErrNotSupported is thrown if listener doesn't support the asked variable
var ErrNotSupported = errors.New("AD: variable not supported by listener")
