// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package listeners

import (
	"errors"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/listeners/listeners_interfaces"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ServiceListenerFactory builds a service listener
type ServiceListenerFactory func(listeners_interfaces.Config) (listeners_interfaces.ServiceListener, error)

// ServiceListenerFactories holds the registered factories
var ServiceListenerFactories = make(map[string]ServiceListenerFactory)

// Register registers a service listener factory
func Register(name string, factory ServiceListenerFactory) {
	if factory == nil {
		log.Warnf("Service listener factory %s does not exist.", name)
		return
	}
	_, registered := ServiceListenerFactories[name]
	if registered {
		log.Errorf("Service listener factory %s already registered. Ignoring.", name)
		return
	}
	ServiceListenerFactories[name] = factory
}

// ErrNotSupported is thrown if listener doesn't support the asked variable
var ErrNotSupported = errors.New("AD: variable not supported by listener")
