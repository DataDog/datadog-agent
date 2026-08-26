// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build jmx

package discoverer

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// jmxBridge implements ConfigDiscoverer for JMX-based integrations.
// Unlike the Python bridge (which calls into the Python rtloader), the JMX
// bridge generates a basic JMX instance config from the service's exposed
// ports. JMXFetch then connects to the JMX endpoint and, when the instance
// carries a "discovery" flag, inspects available MBeans to auto-detect the
// application type (e.g. Kafka, ActiveMQ) and configure application-specific
// metric collection.
type jmxBridge struct{}

// NewJmxBridge returns a ConfigDiscoverer backed by JMXFetch.
func NewJmxBridge() ConfigDiscoverer {
	return &jmxBridge{}
}

// discoveryService is the JSON payload sent to the integration when asking it
// to discover its config for a given service. (Same shape as
// discovery_json.go but re-declared to avoid cross-package visibility issues.)
type jmxDiscoveryService struct {
	ID    string             `json:"id"`
	Host  string             `json:"host"`
	Ports []jmxDiscoveryPort `json:"ports"`
}

type jmxDiscoveryPort struct {
	Number int    `json:"number"`
	Name   string `json:"name"`
}

// discoveredConfig is the JSON shape returned by the integration.
type jmxDiscoveredConfig struct {
	Instances               []json.RawMessage `json:"instances"`
	InitConfig              json.RawMessage   `json:"init_config"`
	MetricConfig            json.RawMessage   `json:"metric_config"`
	LogsConfig              json.RawMessage   `json:"logs"`
	IgnoreAutodiscoveryTags bool              `json:"ignore_autodiscovery_tags"`
	CheckTagCardinality     string            `json:"check_tag_cardinality"`
}

func (b *jmxBridge) DiscoverConfig(integrationName, serviceJSON string) (string, error) {
	// Only handle known JMX integrations
	if !isJMXIntegration(integrationName) {
		return "", fmt.Errorf("not a JMX integration: %s", integrationName)
	}

	var svc jmxDiscoveryService
	if err := json.Unmarshal([]byte(serviceJSON), &svc); err != nil {
		return "", fmt.Errorf("parse service JSON: %w", err)
	}

	jmxPort := findJMXPort(svc.Ports)
	if jmxPort == 0 {
		log.Debugf("JMX discovery: no JMX port found for service %s, skipping", svc.ID)
		return "", fmt.Errorf("no JMX port found for service %s", svc.ID)
	}

	log.Infof("JMX discovery: discovered JMX endpoint %s:%d for integration %s", svc.Host, jmxPort, integrationName)

	// Build a discovered config with the JMX endpoint info.
	// The "discovery" flag tells JMXFetch to inspect MBeans and auto-detect
	// the application type for metric collection.
	instance := map[string]interface{}{
		"host":                        "%%host%%",
		"port":                        jmxPort,
		"collect_default_jvm_metrics": true,
		"discovery":                   true,
	}

	initConfig := map[string]interface{}{
		"is_jmx":                  true,
		"collect_default_metrics": true,
		"new_gc_metrics":          true,
	}

	instanceJSON, _ := json.Marshal(instance)
	initConfigJSON, _ := json.Marshal(initConfig)

	result := []jmxDiscoveredConfig{{
		Instances:  []json.RawMessage{instanceJSON},
		InitConfig: initConfigJSON,
	}}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("marshal discovery result: %w", err)
	}

	log.Debugf("JMX discovery: returning config for %s: %s", integrationName, string(resultJSON))
	return string(resultJSON), nil
}

// findJMXPort finds the JMX port from a list of exposed ports.
// It looks for a port named "jmx" or "jmx-rmi" first, then falls back to
// common JMX ports (9999, 9010, 1099, 7199).
func findJMXPort(ports []jmxDiscoveryPort) int {
	commonJMXPorts := map[int]bool{
		9999: true, // Kafka default
		9010: true, // test app
		1099: true, // JBoss, ActiveMQ
		7199: true, // Cassandra
	}

	// First, look for a port named "jmx" or "jmx-rmi"
	for _, p := range ports {
		if p.Name == "jmx" || p.Name == "jmx-rmi" {
			return p.Number
		}
	}

	// Then, look for common JMX ports
	for _, p := range ports {
		if commonJMXPorts[p.Number] {
			return p.Number
		}
	}

	// If only one port is exposed, use it
	if len(ports) == 1 {
		return ports[0].Number
	}

	return 0
}
