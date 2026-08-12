// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package handlers

import (
	"fmt"
	"hash/fnv"
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
	// trackedServices is the set of "namespace/name" service keys currently targeted by a
	// DDI CR, enabling O(1) HasService lookups. A service is targeted by at most one CR.
	trackedServices map[string]bool
	// subscribers are called with the namespace and name of a service whose templates
	// or tracked-state change.
	subscribers []func(namespace, name string)
}

// NewServiceCheckTemplateStore creates a new ServiceCheckTemplateStore.
func NewServiceCheckTemplateStore() *ServiceCheckTemplateStore {
	return &ServiceCheckTemplateStore{
		entries:         make(map[string]serviceTemplateEntry),
		trackedServices: make(map[string]bool),
	}
}

// NotifyOnChange registers a callback invoked with the namespace and name of a
// service whose templates or tracked-state change. Multiple subscribers are supported.
func (s *ServiceCheckTemplateStore) NotifyOnChange(fn func(namespace, name string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subscribers = append(s.subscribers, fn)
}

// writeTemplates stores templates keyed by DDI CR,
// associating them with the target service from the CR's TargetRef.
func (s *ServiceCheckTemplateStore) writeTemplates(crKey string, cr *datadoghq.DatadogInstrumentation, configs []integration.Config) {
	svcKey := cr.Namespace + "/" + cr.Spec.TargetRef.Name

	s.mu.Lock()
	_, existed := s.entries[crKey]
	if len(configs) == 0 {
		delete(s.entries, crKey)
		delete(s.trackedServices, svcKey)
	} else {
		s.entries[crKey] = serviceTemplateEntry{
			serviceNamespace: cr.Namespace,
			serviceName:      cr.Spec.TargetRef.Name,
			templates:        configs,
		}
		s.trackedServices[svcKey] = true
	}
	s.mu.Unlock()

	// Nothing changed if there was neither a prior entry nor new templates.
	if existed || len(configs) > 0 {
		s.notify(cr.Namespace, cr.Spec.TargetRef.Name)
	}
}

// deleteTemplates removes templates keyed by DDI CR.
func (s *ServiceCheckTemplateStore) deleteTemplates(crKey string) {
	s.mu.Lock()
	old, exists := s.entries[crKey]
	if exists {
		delete(s.entries, crKey)
		delete(s.trackedServices, old.serviceNamespace+"/"+old.serviceName)
	}
	s.mu.Unlock()

	if exists {
		s.notify(old.serviceNamespace, old.serviceName)
	}
}

// notify fans out the affected service to all registered subscribers. It must be
// called without holding s.mu so subscribers can safely call back into the store.
func (s *ServiceCheckTemplateStore) notify(namespace, name string) {
	s.mu.RLock()
	subscribers := append([]func(namespace, name string){}, s.subscribers...)
	s.mu.RUnlock()
	for _, fn := range subscribers {
		fn(namespace, name)
	}
}

// HasService reports whether a DDI CR's templates target the given service.
func (s *ServiceCheckTemplateStore) HasService(namespace, name string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.trackedServices[namespace+"/"+name]
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
		configs: make(map[string][]integration.Config),
		states:  make(map[string]string),
		// configHash is the XOR of all per-entry hashes; the empty set hashes to 0.
		configHash: 0,
	}
}

// ListConfigs returns a snapshot of all stored integration.Config entries and the
// current state hash.
func (c *CheckStore) ListConfigs() ([]integration.Config, uint64) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var out []integration.Config
	for _, cfgs := range c.configs {
		out = append(out, cfgs...)
	}
	return out, c.configHash
}

// Hash returns a deterministic hash of the current set of "key:uid:generation" entries,
// consistent across all cluster agent replicas for the same CR state.
func (c *CheckStore) Hash() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.configHash
}

func (c *CheckStore) writeConfigs(key string, cr *datadoghq.DatadogInstrumentation, configs []integration.Config) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Remove the previous entry's contribution before applying the change, then add the
	// new one.
	if old, ok := c.states[key]; ok {
		c.configHash ^= entryHash(key, old)
	}
	if len(configs) == 0 {
		delete(c.configs, key)
		delete(c.states, key)
	} else {
		c.configs[key] = configs
		state := fmt.Sprintf("%s:%d", cr.UID, cr.Generation)
		c.states[key] = state
		c.configHash ^= entryHash(key, state)
	}
}

func (c *CheckStore) deleteConfigs(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if old, ok := c.states[key]; ok {
		c.configHash ^= entryHash(key, old)
	}
	delete(c.configs, key)
	delete(c.states, key)
}

// entryHash returns the hash contribution of a single "key/state" entry. writeConfigs and
// deleteConfigs maintain CheckStore.configHash by XOR-combining these per-entry hashes; XOR is
// commutative, so the result is independent of the order entries are applied and identical across
// cluster agent replicas with the same CR state.
func entryHash(key, state string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	// The 0 separator keeps ("ab", "c") distinct from ("a", "bc").
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(state))
	return h.Sum64()
}

func isService(cr *datadoghq.DatadogInstrumentation) bool {
	return cr != nil && cr.Spec.TargetRef.Kind == "Service"
}
