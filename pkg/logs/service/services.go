// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package service

import (
	"sync"
)

// Services provides new and removed services.
type Services struct {
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
	s.mu.Lock()
	defer s.mu.Unlock()

	added, _ := s.addedPerType[service.Type]
	for _, ch := range append(added, s.allAdded...) {
		ch <- service
	}
}

// RemoveService sends a removed service to all the channels that registered.
func (s *Services) RemoveService(service *Service) {
	s.mu.Lock()
	defer s.mu.Unlock()

	removed, _ := s.removedPerType[service.Type]
	for _, ch := range append(removed, s.allRemoved...) {
		ch <- service
	}
}

// GetAddedServicesForType returns a stream of new services for a given type.
func (s *Services) GetAddedServicesForType(serviceType string) chan *Service {
	s.mu.Lock()
	defer s.mu.Unlock()

	added := make(chan *Service)
	s.addedPerType[serviceType] = append(s.addedPerType[serviceType], added)

	return added
}

// GetRemovedServicesForType returns a stream of removed services for a given type.
func (s *Services) GetRemovedServicesForType(serviceType string) chan *Service {
	s.mu.Lock()
	defer s.mu.Unlock()

	removed := make(chan *Service)
	s.removedPerType[serviceType] = append(s.removedPerType[serviceType], removed)

	return removed
}

// GetAllAddedServices registers the channel to receive all added services.
func (s *Services) GetAllAddedServices() chan *Service {
	s.mu.Lock()
	defer s.mu.Unlock()

	added := make(chan *Service)
	s.allAdded = append(s.allAdded, added)

	return added
}

// GetAllRemovedServices registers the channel to receive all removed services.
func (s *Services) GetAllRemovedServices() chan *Service {
	s.mu.Lock()
	defer s.mu.Unlock()

	removed := make(chan *Service)
	s.allRemoved = append(s.allRemoved, removed)

	return removed
}
