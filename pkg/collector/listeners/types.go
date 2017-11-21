// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package listeners

import log "github.com/cihub/seelog"

// ID is the representation of the unique ID of a Service
type ID string

// Service represents an application we can run a check against.
// It should be matched with a check template by the ConfigResolver using the
// ADIdentifiers field.
type Service interface {
	GetID() ID                            // unique ID
	GetADIdentifiers() ([]string, error)  // identifiers on which templates will be matched
	GetHosts() (map[string]string, error) // network --> IP address
	GetPorts() ([]int, error)             // network ports
	GetTags() ([]string, error)           // tags
	GetPid() (int, error)                 // process identifier
}

// ServiceListener monitors running services and triggers check (un)scheduling
//
// It holds a cache of running services, listens to new/killed services and
// updates its cache, and the ConfigResolver with these events.
type ServiceListener interface {
	Listen(newSvc chan<- Service, delSvc chan<- Service)
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
