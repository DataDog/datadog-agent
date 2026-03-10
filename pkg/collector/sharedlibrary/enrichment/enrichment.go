// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package enrichment provides agent metadata serialization for shared library checks.
// The Go structs defined here mirror the Rust EnrichmentData struct and are serialized
// to YAML for passing enrichment data across the FFI boundary.
package enrichment

import (
	yaml "go.yaml.in/yaml/v2"
)

// EnrichmentData contains agent metadata passed to shared library (Rust) checks.
// Field names and types must match the Rust EnrichmentData struct in
// pkg/collector/sharedlibrary/rustchecks/core/src/enrichment.rs
type EnrichmentData struct {
	Hostname          string                 `yaml:"hostname"`
	HostTags          map[string]string      `yaml:"host_tags"`
	ClusterName       *string                `yaml:"cluster_name,omitempty"`
	AgentVersion      string                 `yaml:"agent_version"`
	ConfigValues      map[string]any `yaml:"config_values"`
	ProcessStartTime  uint64                 `yaml:"process_start_time"`
	K8sConnectionInfo *K8sConnectionInfo     `yaml:"k8s_connection_info,omitempty"`
}

// K8sConnectionInfo contains Kubernetes connection details.
type K8sConnectionInfo struct {
	APIServerURL string  `yaml:"api_server_url"`
	BearerToken  *string `yaml:"bearer_token,omitempty"`
}

// Provider is the interface for obtaining enrichment YAML to pass to Rust checks.
type Provider interface {
	GetEnrichmentYAML() string
}

// StaticProvider is a Provider that returns pre-built enrichment YAML.
// It is constructed once at initialization time with a snapshot of agent metadata.
type StaticProvider struct {
	yaml string
}

// NewStaticProvider creates a StaticProvider by serializing the given EnrichmentData to YAML.
func NewStaticProvider(data EnrichmentData) (*StaticProvider, error) {
	// Ensure maps are non-nil so YAML serialization produces "{}" instead of "null"
	if data.HostTags == nil {
		data.HostTags = map[string]string{}
	}
	if data.ConfigValues == nil {
		data.ConfigValues = map[string]any{}
	}

	yamlBytes, err := yaml.Marshal(data)
	if err != nil {
		return nil, err
	}
	return &StaticProvider{yaml: string(yamlBytes)}, nil
}

// GetEnrichmentYAML returns the pre-serialized enrichment YAML string.
func (p *StaticProvider) GetEnrichmentYAML() string {
	return p.yaml
}
