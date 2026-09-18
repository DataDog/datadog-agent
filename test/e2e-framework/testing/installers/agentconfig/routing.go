// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentconfig

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/receivers"
	"go.yaml.in/yaml/v3"
)

// GenerateWithRouting renders an explicit route after rejecting raw credential,
// destination, trust, and unsupported backend settings. Secrets are transient
// input, never part of the public plan. Unrelated collection settings survive.
func GenerateWithRouting(p receivers.Plan, apiKey, extraConfig string) (string, error) {
	if err := p.Validate(); err != nil {
		return "", err
	}
	if _, err := receivers.ValidateConfig(extraConfig); err != nil {
		return "", err
	}
	// Render from the original tree, not the flattened validation projection.
	// Map-valued settings (e.g. container_env_as_tags) have literal, case-sensitive
	// keys. Flattening them both loses siblings in the Agent reader and changes
	// the meaning of dictionary keys containing dots.
	m := map[string]any{}
	if err := yaml.Unmarshal([]byte(extraConfig), &m); err != nil {
		return "", fmt.Errorf("Agent config: invalid YAML")
	}
	if p.APIKeyRef == "" {
		apiKey = receivers.DummyAPIKey
	}
	if apiKey == "" {
		return "", fmt.Errorf("receiver API credential is empty")
	}
	for k, v := range p.Settings() {
		m[k] = v
	}
	m["api_key"] = apiKey
	data, err := yaml.Marshal(m)
	return string(data), err
}
