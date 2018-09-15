// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package service

import (
	"sync"
)

// Services provides new and removed services.
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

// AddService sends a new service to the channel matching its type.
func (s *Services) AddService(service *Service) {
	s.mu.Lock()
	added, exists := s.addedPerType[service.Type]
	s.mu.Unlock()

	if exists {
		added <- service
	}
}

// RemoveService sends a removed service to the channel matching its type.
func (s *Services) RemoveService(service *Service) {
	s.mu.Lock()
	removed, exists := s.removedPerType[service.Type]
	s.mu.Unlock()

	if exists {
		removed <- service
	}
}

// GetAddedServices returns a stream of new services for a given type.
func (s *Services) GetAddedServices(serviceType string) chan *Service {
	s.mu.Lock()
	defer s.mu.Unlock()

	added, exists := s.addedPerType[serviceType]
	if !exists {
		added = make(chan *Service)
		s.addedPerType[serviceType] = added
	}
	return added
}

// GetRemovedServices returns a stream of removed services for a given type.
func (s *Services) GetRemovedServices(serviceType string) chan *Service {
	s.mu.Lock()
	defer s.mu.Unlock()

	removed, exists := s.removedPerType[serviceType]
	if !exists {
		removed = make(chan *Service)
		s.removedPerType[serviceType] = removed
	}
	return removed
}
