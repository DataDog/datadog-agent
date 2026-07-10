// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package datasecurityimpl

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	yaml "gopkg.in/yaml.v3"
)

type backendKind string

const (
	backendPostgres backendKind = "postgres"
)

// scanBackend is the extension point for data-security scan engines in the Go
// component. Register new engines in scanBackends below.
type scanBackend interface {
	Kind() backendKind
	IntegrationName() string
	Detect(payload rcPayload) bool
	ConnectHost(instance map[string]any) string
	ApplyRuntimeConnection(instance map[string]any, runtimeCfg *runtimeInstanceConfig) error
}

// scanBackends is the single registry of supported backends. Add a new engine
// here and implement scanBackend in backend_<name>.go.
var scanBackends = []scanBackend{
	postgresBackend{},
}

func backendByKind(kind backendKind) (scanBackend, bool) {
	for _, backend := range scanBackends {
		if backend.Kind() == kind {
			return backend, true
		}
	}
	return nil, false
}

func detectBackend(payload rcPayload) (backendKind, error) {
	for _, backend := range scanBackends {
		if backend.Detect(payload) {
			return backend.Kind(), nil
		}
	}
	return "", errNoScanData
}

func (c *component) findIntegrationInstance(kind backendKind) (map[string]any, integration.Config, bool) {
	backend, ok := backendByKind(kind)
	if !ok {
		return nil, integration.Config{}, false
	}

	for _, cfg := range c.ac.GetUnresolvedConfigs() {
		if cfg.Name != backend.IntegrationName() {
			continue
		}
		for _, instanceData := range cfg.Instances {
			var instance map[string]any
			if err := yaml.Unmarshal(instanceData, &instance); err != nil {
				continue
			}
			if !instanceDataSecurityEnabled(instance) {
				continue
			}

			c.log.Infof("datasecurity: using %s instance (connect_host=%q reported_hostname=%q)",
				backend.IntegrationName(),
				backend.ConnectHost(instance),
				stringField(instance, "reported_hostname"))
			return instance, cfg, true
		}
	}

	return nil, integration.Config{}, false
}

func (c *component) hasEligibleIntegration() bool {
	for _, backend := range scanBackends {
		if _, _, ok := c.findIntegrationInstance(backend.Kind()); ok {
			return true
		}
	}
	return false
}

func applyRuntimeConnection(
	kind backendKind,
	instance map[string]any,
	runtimeCfg *runtimeInstanceConfig,
) error {
	backend, ok := backendByKind(kind)
	if !ok {
		return fmt.Errorf("unsupported backend %q", kind)
	}
	return backend.ApplyRuntimeConnection(instance, runtimeCfg)
}

func portIntWithDefault(instance map[string]any, defaultPort int) int {
	switch v := instance["port"].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		return atoiOrDefault(v, defaultPort)
	}
	return defaultPort
}

func atoiOrDefault(value string, defaultPort int) int {
	port, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return defaultPort
	}
	return port
}
