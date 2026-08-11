// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	yaml "go.yaml.in/yaml/v2"
)

// TestDefinition is the YAML input describing what to provision, install,
// and run. It is the "what you want" half of e2ectl's two-file split — the
// state file (see state.go) is the generated "what exists" half.
type TestDefinition struct {
	Name        string            `yaml:"name"`
	Provisioner provisionerConfig `yaml:"provisioner"`
	Agent       agentConfig       `yaml:"agent"`
	Test        testConfig        `yaml:"test"`
}

type provisionerConfig struct {
	Type    string         `yaml:"type"`
	Options map[string]any `yaml:"options"`
}

type agentConfig struct {
	Installer           string `yaml:"installer"`
	AgentVersion        string `yaml:"agentVersion"`
	ClusterAgentVersion string `yaml:"clusterAgentVersion"`
	Namespace           string `yaml:"namespace"`
}

type testConfig struct {
	Package string `yaml:"package"`
	Run     string `yaml:"run"`
}

// loadTestDefinition reads and validates a TestDefinition from a YAML file.
func loadTestDefinition(path string) (TestDefinition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return TestDefinition{}, err
	}

	var def TestDefinition
	if err := yaml.Unmarshal(data, &def); err != nil {
		return TestDefinition{}, fmt.Errorf("parsing %s: %w", path, err)
	}

	if strings.TrimSpace(def.Name) == "" {
		return TestDefinition{}, fmt.Errorf("%s: name is required", path)
	}
	if strings.TrimSpace(def.Provisioner.Type) == "" {
		return TestDefinition{}, fmt.Errorf("%s: provisioner.type is required", path)
	}
	return def, nil
}

// provisionConfig converts the YAML provisioner section into the
// json.RawMessage-based shape provisioners.go's registry expects — the
// registry's factory functions are unchanged from cmd/envctl, so this is
// the only place the YAML->JSON bridging happens.
func (def TestDefinition) provisionConfig() (provisionConfig, error) {
	options, err := json.Marshal(def.Provisioner.Options)
	if err != nil {
		return provisionConfig{}, fmt.Errorf("marshaling provisioner options: %w", err)
	}
	return provisionConfig{Provisioner: def.Provisioner.Type, Options: options}, nil
}

// stateDirFor returns the centralized, gitignored directory e2ectl stores
// every environment's state file in, rooted at root (the repository root —
// see repoRoot in wizard.go). Centralizing means the dashboard
// (dashboard.go, Task 5) can discover every environment e2ectl knows about
// regardless of which directory it was invoked from or which YAML produced
// each one.
func stateDirFor(root string) string {
	return filepath.Join(root, "test", "e2e-framework", ".e2ectl-state")
}

// defaultStatePath returns where name's state file lives by default. name
// is the TestDefinition's `name:` field, already required to be unique
// (it's reused for the kind cluster name, the fakeintake container name,
// and the Helm release namespace/labels), so it doubles as a safe
// state-file key.
func defaultStatePath(root, name string) string {
	return filepath.Join(stateDirFor(root), name+".state.json")
}
