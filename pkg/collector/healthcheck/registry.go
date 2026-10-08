// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package healthcheck observes integration health checks and dispatches remediation requests locally.
package healthcheck

import (
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
)

const (
	defaultCooldown    = 5 * time.Minute
	defaultMaxAttempts = 3
)

// maxTrackedContexts bounds the per-check status map so a high-cardinality wildcard service check
// (many host/tag contexts) cannot grow it without limit; the oldest context is evicted when full.
const maxTrackedContexts = 1024

type contextStatus struct {
	status servicecheck.ServiceCheckStatus
	at     time.Time
}

type registration struct {
	generation  uint64
	config      *integration.HealthCheckConfig
	cooldown    time.Duration
	maxAttempts int
	statuses    map[string]contextStatus
	windowStart time.Time
	attempts    int
}

var registrationGeneration atomic.Uint64

var registry = struct {
	sync.RWMutex
	checks map[checkid.ID]*registration
}{checks: make(map[checkid.ID]*registration)}

// Register stores a private copy of an enabled declaration and resets its observation state.
func Register(id checkid.ID, cfg *integration.HealthCheckConfig) {
	if id == "" || cfg == nil || !cfg.Enabled {
		return
	}
	cooldown, err := time.ParseDuration(cfg.Remediation.Cooldown)
	if err != nil || cooldown <= 0 {
		cooldown = defaultCooldown
	}
	maxAttempts := cfg.Remediation.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = defaultMaxAttempts
	}
	entry := &registration{
		config:      cloneConfig(cfg),
		cooldown:    cooldown,
		maxAttempts: maxAttempts,
		statuses:    make(map[string]contextStatus),
	}
	registry.Lock()
	defer registry.Unlock()
	entry.generation = registrationGeneration.Add(1)
	registry.checks[id] = entry
}

// Unregister removes a check's declaration and all transition and attempt state.
func Unregister(id checkid.ID) {
	registry.Lock()
	defer registry.Unlock()
	delete(registry.checks, id)
}

// Lookup returns a private copy of a registered declaration, or nil and false on a miss.
func Lookup(id checkid.ID) (*integration.HealthCheckConfig, bool) {
	registry.RLock()
	defer registry.RUnlock()
	entry, ok := registry.checks[id]
	if !ok {
		return nil, false
	}
	return cloneConfig(entry.config), true
}

func cloneConfig(cfg *integration.HealthCheckConfig) *integration.HealthCheckConfig {
	copy := *cfg
	copy.Remediation.Steps = slices.Clone(cfg.Remediation.Steps)
	copy.Remediation.AllowedPaths = slices.Clone(cfg.Remediation.AllowedPaths)
	if cfg.Remediation.AllowedServices != nil {
		copy.Remediation.AllowedServices = make(map[string][]string, len(cfg.Remediation.AllowedServices))
		for service, actions := range cfg.Remediation.AllowedServices {
			copy.Remediation.AllowedServices[service] = slices.Clone(actions)
		}
	}
	return &copy
}
