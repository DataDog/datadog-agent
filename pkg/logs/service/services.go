// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package service

import (
	"sync"
)

// capacity of the channels
const capacity = 10

// Services provides new and removed services,
type Services struct {
	addedPerType   map[string]chan *Service
	removedPerType map[string]chan *Service
	mu             sync.Mutex
}

// NewServices returns a new services.
func NewServices() *Services {
	return &Services{
		addedPerType:   make(map[string]chan *Service),
		removedPerType: make(map[string]chan *Service),
	}
}

// AddService sends a new service to the proper channel.
func (s *Services) AddService(service *Service) {
	s.getAddedServices(service.Type) <- service
}

// RemoveService sends a removed service to the proper channel.
func (s *Services) RemoveService(service *Service) {
	s.getRemovedServices(service.Type) <- service
}

// GetAddedServices returns a stream of new services for a given type.
func (s *Services) GetAddedServices(serviceType string) chan *Service {
	return s.getAddedServices(serviceType)
}

// GetRemovedServices returns a stream of removed services for a given type.
func (s *Services) GetRemovedServices(serviceType string) chan *Service {
	return s.getRemovedServices(serviceType)
}

func (s *Services) getAddedServices(serviceType string) chan *Service {
	s.mu.Lock()
	defer s.mu.Unlock()
	if added, exists := s.addedPerType[serviceType]; exists {
		return added
	}
	added := make(chan *Service, capacity)
	s.addedPerType[serviceType] = added
	return added
}

func (s *Services) getRemovedServices(serviceType string) chan *Service {
	s.mu.Lock()
	defer s.mu.Unlock()
	if removed, exists := s.removedPerType[serviceType]; exists {
		return removed
	}
	removed := make(chan *Service, capacity)
	s.removedPerType[serviceType] = removed
	return removed
}
