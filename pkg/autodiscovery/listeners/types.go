// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package listeners

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ID is the representation of the unique ID of a Service
type ID string

// ContainerPort represents a network port in a Service.
type ContainerPort struct {
	Port int
	Name string
}

// Service represents an application we can run a check against.
// It should be matched with a check template by the ConfigResolver using the
// ADIdentifiers field.
type Service interface {
	GetEntity() string                         // unique entity name
	GetTaggerEntity() string                   // tagger entity name
	GetADIdentifiers() ([]string, error)       // identifiers on which templates will be matched
	GetHosts() (map[string]string, error)      // network --> IP address
	GetPorts() ([]ContainerPort, error)        // network ports
	GetTags() ([]string, error)                // tags
	GetPid() (int, error)                      // process identifier
	GetHostname() (string, error)              // hostname.domainname for the entity
	GetCreationTime() integration.CreationTime // created before or after the agent start
	IsReady() bool                             // is the service ready
	GetCheckNames() []string                   // slice of check names defined in kubernetes annotations or docker labels
	HasFilter(containers.FilterType) bool      // whether the service is excluded by metrics or logs exclusion config
	GetExtraConfig(string) (string, error)     // Extra configuration values
}

// ServiceListener monitors running services and triggers check (un)scheduling
//
// It holds a cache of running services, listens to new/killed services and
// updates its cache, and the AutoConfig with these events.
type ServiceListener interface {
	Listen(newSvc, delSvc chan<- Service)
	Stop()
}

// ServiceListenerFactory builds a service listener
type ServiceListenerFactory func() (ServiceListener, error)

// ServiceListenerFactories holds the registered factories
var ServiceListenerFactories = make(map[string]ServiceListenerFactory)

// Register registers a service listener factory
func Register(name string, factory ServiceListenerFactory) {
	if factory == nil {
		log.Warnf("Service listener factory %s does not exist.", name)
	}
	_, registered := ServiceListenerFactories[name]
	if registered {
		log.Errorf("Service listener factory %s already registered. Ignoring.", name)
	}
	ServiceListenerFactories[name] = factory
}

// ErrNotSupported is thrown if listener doesn't support the asked variable
var ErrNotSupported = errors.New("AD: variable not supported by listener")
