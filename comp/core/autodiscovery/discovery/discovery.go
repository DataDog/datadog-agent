// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package discovery provides discovery information storage and lookup for autodiscovery.
package discovery

import (
	"sync"

	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Info contains the discovery information parsed from configuration files
type Info struct {
	LogSource string `yaml:"log_source,omitempty"`
}

// Registry stores discovery information indexed by AD identifiers
type Registry struct {
	mu           sync.RWMutex
	discoveryMap map[string]Info
}

var registry *Registry
var registryOnce sync.Once

// GetRegistry returns the singleton discovery registry
func GetRegistry() *Registry {
	registryOnce.Do(func() {
		registry = &Registry{
			discoveryMap: make(map[string]Info),
		}
	})
	return registry
}

// RegisterConfig registers discovery information from a configuration
func (r *Registry) RegisterConfig(config integration.Config) {
	if config.DiscoveryConfig == nil {
		return
	}

	var discoveryInfo Info
	if err := yaml.Unmarshal(config.DiscoveryConfig, &discoveryInfo); err != nil {
		log.Warnf("Failed to parse discovery config for %s: %v", config.Name, err)
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, identifier := range config.ADIdentifiers {
		if identifier != "" {
			r.discoveryMap[identifier] = discoveryInfo
			log.Debugf("Registered discovery info for identifier '%s': log_source=%s", identifier, discoveryInfo.LogSource)
		}
	}
}

// GetLogSource returns the configured log source for the given AD identifier
func (r *Registry) GetLogSource(identifier string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	info, exists := r.discoveryMap[identifier]
	if !exists || info.LogSource == "" {
		return "", false
	}
	return info.LogSource, true
}

// Reset clears all registered discovery information
func (r *Registry) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.discoveryMap = make(map[string]Info)
}
