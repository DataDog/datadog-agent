// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package autodiscovery

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
)

// store holds useful mappings for the AD
type store struct {
	serviceToConfigs  map[string][]integration.Config
	serviceToTagsHash map[string]string
	loadedConfigs     map[string]integration.Config
	nameToJMXMetrics  map[string]integration.Data
	m                 sync.RWMutex
}

// newStore creates a store
func newStore() *store {
	s := store{
		serviceToConfigs:  make(map[string][]integration.Config),
		serviceToTagsHash: make(map[string]string),
		loadedConfigs:     make(map[string]integration.Config),
		nameToJMXMetrics:  make(map[string]integration.Data),
	}

	return &s
}

// getConfigsForService gets config for a specified service
func (s *store) getConfigsForService(serviceID string) []integration.Config {
	s.m.RLock()
	defer s.m.RUnlock()
	return s.serviceToConfigs[serviceID]
}

// removeConfigsForService removes a config for a specified service
func (s *store) removeConfigsForService(serviceID string) {
	s.m.Lock()
	defer s.m.Unlock()
	delete(s.serviceToConfigs, serviceID)
}

// addConfigForService adds a config for a specified service
func (s *store) addConfigForService(serviceID string, config integration.Config) {
	s.m.Lock()
	defer s.m.Unlock()
	existingConfigs, found := s.serviceToConfigs[serviceID]
	if found {
		s.serviceToConfigs[serviceID] = append(existingConfigs, config)
	} else {
		s.serviceToConfigs[serviceID] = []integration.Config{config}
	}
}

// getTagsHashForService return the tags hash for a specified service
func (s *store) getTagsHashForService(serviceID string) string {
	s.m.RLock()
	defer s.m.RUnlock()
	return s.serviceToTagsHash[serviceID]
}

// removeTagsHashForService removes the tags hash for a specified service
func (s *store) removeTagsHashForService(serviceID string) {
	s.m.Lock()
	defer s.m.Unlock()
	delete(s.serviceToTagsHash, serviceID)
}

// setTagsHashForService set the tags hash for a specified service
func (s *store) setTagsHashForService(serviceID string, hash string) {
	s.m.Lock()
	defer s.m.Unlock()
	s.serviceToTagsHash[serviceID] = hash
}

// setLoadedConfig stores a resolved config by its digest
func (s *store) setLoadedConfig(config integration.Config) {
	s.m.Lock()
	defer s.m.Unlock()
	s.loadedConfigs[config.Digest()] = config
}

// removeLoadedConfig removes a loaded config by its digest
func (s *store) removeLoadedConfig(config integration.Config) {
	s.m.Lock()
	defer s.m.Unlock()
	delete(s.loadedConfigs, config.Digest())
}

// getLoadedConfigs returns all loaded and resolved configs
func (s *store) getLoadedConfigs() map[string]integration.Config {
	s.m.RLock()
	defer s.m.RUnlock()
	return s.loadedConfigs
}

// setJMXMetricsForConfigName stores the jmx metrics config for a config name
func (s *store) setJMXMetricsForConfigName(config string, metrics integration.Data) {
	s.m.Lock()
	defer s.m.Unlock()
	s.nameToJMXMetrics[config] = metrics
}

// getJMXMetricsForConfigName returns the stored JMX metrics for a config name
func (s *store) getJMXMetricsForConfigName(config string) integration.Data {
	s.m.RLock()
	defer s.m.RUnlock()
	return s.nameToJMXMetrics[config]
}
