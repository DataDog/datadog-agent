// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package handlers

import (
	"fmt"
	"hash/fnv"
	"sort"
	"sync"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

// serviceTemplateEntry links a DDI CR key to a target service and its check templates.
type serviceTemplateEntry struct {
	serviceNamespace string
	serviceName      string
	templates        []integration.Config
}

// ServiceCheckTemplateStore holds check templates for Service-targeted DDI CRs.
// The handler writes templates here; a separate AD config provider reads them and
// combines with EndpointSlice data to produce per-endpoint configs.
type ServiceCheckTemplateStore struct {
	mu sync.RWMutex
	// entries maps DDI CR key (namespace/name) to the target service and templates.
	entries map[string]serviceTemplateEntry
	// onChange is called when templates are added or removed.
	onChange func()
}

// NewServiceCheckTemplateStore creates a new ServiceCheckTemplateStore.
func NewServiceCheckTemplateStore() *ServiceCheckTemplateStore {
	return &ServiceCheckTemplateStore{
		entries: make(map[string]serviceTemplateEntry),
	}
}

// SetOnChange registers a callback invoked when the template set changes.
func (s *ServiceCheckTemplateStore) SetOnChange(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onChange = fn
}

// writeTemplates stores templates keyed by DDI CR,
// associating them with the target service from the CR's TargetRef.
func (s *ServiceCheckTemplateStore) writeTemplates(key string, cr *datadoghq.DatadogInstrumentation, configs []integration.Config) {
	s.mu.Lock()
	if len(configs) == 0 {
		delete(s.entries, key)
	} else {
		s.entries[key] = serviceTemplateEntry{
			serviceNamespace: cr.Namespace,
			serviceName:      cr.Spec.TargetRef.Name,
			templates:        configs,
		}
	}
	onChange := s.onChange
	s.mu.Unlock()
	if onChange != nil {
		onChange()
	}
}

func (s *ServiceCheckTemplateStore) deleteTemplates(key string) {
	s.mu.Lock()
	delete(s.entries, key)
	onChange := s.onChange
	s.mu.Unlock()
	if onChange != nil {
		onChange()
	}
}

// HasService reports whether any templates target the given service.
func (s *ServiceCheckTemplateStore) HasService(namespace, name string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, entry := range s.entries {
		if entry.serviceNamespace == namespace && entry.serviceName == name {
			return true
		}
	}
	return false
}

// AllTemplatesByService returns all templates grouped by "namespace/name" service key
// in a single pass. This avoids repeated lock acquisitions per service.
func (s *ServiceCheckTemplateStore) AllTemplatesByService() map[string][]integration.Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string][]integration.Config)
	for _, entry := range s.entries {
		key := entry.serviceNamespace + "/" + entry.serviceName
		out[key] = append(out[key], entry.templates...)
	}
	return out
}

// templatesForService returns all check templates targeting a given service.
func (s *ServiceCheckTemplateStore) templatesForService(namespace, name string) []integration.Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []integration.Config
	for _, entry := range s.entries {
		if entry.serviceNamespace == namespace && entry.serviceName == name {
			out = append(out, entry.templates...)
		}
	}
	return out
}

// CheckStore stores integration.Config entries keyed by DatadogInstrumentation CR.
type CheckStore struct {
	mu      sync.RWMutex
	configs map[string][]integration.Config
	// states maps namespace/name → "uid:generation". Including the UID ensures that a
	// delete+recreate of a CR with the same name is detected even when the new CR
	// starts at generation 1.
	states     map[string]string
	configHash uint64
}

// NewCheckStore creates a new CheckStore.
func NewCheckStore() *CheckStore {
	return &CheckStore{
		configs:    make(map[string][]integration.Config),
		states:     make(map[string]string),
		configHash: fnv.New64a().Sum64(),
	}
}

// ListConfigs returns a snapshot of all stored integration.Config entries and the
// current state hash.
func (c *CheckStore) ListConfigs() ([]integration.Config, uint64) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]integration.Config, 0)
	for _, cfgs := range c.configs {
		out = append(out, cfgs...)
	}
	return out, c.configHash
}

// Hash returns a deterministic hash of the current set of (key, generation) pairs,
// consistent across all cluster agent replicas for the same CR state.
func (c *CheckStore) Hash() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.configHash
}

func (c *CheckStore) writeConfigs(key string, cr *datadoghq.DatadogInstrumentation, configs []integration.Config) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(configs) == 0 {
		delete(c.configs, key)
		delete(c.states, key)
	} else {
		c.configs[key] = configs
		c.states[key] = fmt.Sprintf("%s:%d", cr.UID, cr.Generation)
	}
	c.configHash = c.hashStates()
}

func (c *CheckStore) deleteConfigs(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.configs, key)
	delete(c.states, key)
	c.configHash = c.hashStates()
}

// hashStates computes a deterministic hash from the sorted set of
// "key:uid:generation" entries in the store. Including the UID ensures that a
// recreation of a CR with the same namespace/name is detected even when
// the new CR starts at generation 1.
func (c *CheckStore) hashStates() uint64 {
	keys := make([]string, 0, len(c.states))
	for k := range c.states {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := fnv.New64a()
	for _, k := range keys {
		fmt.Fprintf(h, "%s:%s\n", k, c.states[k]) //nolint:errcheck
	}
	return h.Sum64()
}

func isService(cr *datadoghq.DatadogInstrumentation) bool {
	return cr.Spec.TargetRef.Kind == "Service"
}
