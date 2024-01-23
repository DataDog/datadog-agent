// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package service

import (
	"sync"
)

// Services provides new and removed services.
type Services struct {
	services       []*Service
	addedPerType   map[string][]chan *Service
	removedPerType map[string][]chan *Service
	allAdded       []chan *Service
	allRemoved     []chan *Service
	mu             sync.Mutex
}

// NewServices returns a new services.
func NewServices() *Services {
	return &Services{
		addedPerType:   make(map[string][]chan *Service),
		removedPerType: make(map[string][]chan *Service),
	}
}

// AddService sends a new service to all the channels that registered.
func (s *Services) AddService(service *Service) {
	panic("not called")
}

// RemoveService sends a removed service to all the channels that registered.
func (s *Services) RemoveService(service *Service) {
	panic("not called")
}

// GetAddedServicesForType returns a stream of new services for a given type.
//
// Any services added before this call are delivered from a new goroutine.
func (s *Services) GetAddedServicesForType(serviceType string) chan *Service {
	panic("not called")
}

// GetRemovedServicesForType returns a stream of removed services for a given type.
func (s *Services) GetRemovedServicesForType(serviceType string) chan *Service {
	panic("not called")
}

// GetAllAddedServices registers the channel to receive all added services.
//
// Any services added before this call are delivered from a new goroutine.
func (s *Services) GetAllAddedServices() chan *Service {
	panic("not called")
}

// GetAllRemovedServices registers the channel to receive all removed services.
func (s *Services) GetAllRemovedServices() chan *Service {
	panic("not called")
}
