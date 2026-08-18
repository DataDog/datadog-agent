// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleYAML = `
name: kind-nopulumi-demo
provisioner:
  type: kind
  options:
    kubeVersion: "1.31"
    withoutFakeIntake: false
agent:
  installer: helm-k8s
  agentVersion: latest
  clusterAgentVersion: latest
  namespace: datadog
test:
  package: ./examples/...
  run: TestKindNoPulumi
`

func writeTempYAML(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func TestLoadTestDefinition(t *testing.T) {
	path := writeTempYAML(t, sampleYAML)

	def, err := loadTestDefinition(path)
	require.NoError(t, err)

	assert.Equal(t, "kind-nopulumi-demo", def.Name)
	assert.Equal(t, "kind", def.Provisioner.Type)
	assert.Equal(t, "1.31", def.Provisioner.Options["kubeVersion"])
	assert.Equal(t, "helm-k8s", def.Agent.Installer)
	assert.Equal(t, "./examples/...", def.Test.Package)
	assert.Equal(t, "TestKindNoPulumi", def.Test.Run)
}

func TestLoadTestDefinitionRequiresName(t *testing.T) {
	path := writeTempYAML(t, `
provisioner:
  type: kind
`)
	_, err := loadTestDefinition(path)
	assert.ErrorContains(t, err, "name is required")
}

func TestLoadTestDefinitionRequiresProvisionerType(t *testing.T) {
	path := writeTempYAML(t, `
name: foo
`)
	_, err := loadTestDefinition(path)
	assert.ErrorContains(t, err, "provisioner.type is required")
}

func TestTestDefinitionProvisionConfig(t *testing.T) {
	path := writeTempYAML(t, sampleYAML)
	def, err := loadTestDefinition(path)
	require.NoError(t, err)

	cfg, err := def.provisionConfig()
	require.NoError(t, err)
	assert.Equal(t, "kind", cfg.Provisioner)
	assert.JSONEq(t, `{"kubeVersion":"1.31","withoutFakeIntake":false}`, string(cfg.Options))
}

func TestStateDirFor(t *testing.T) {
	assert.Equal(t, filepath.Join("/repo", "test", "e2e-framework", ".e2ectl-state"), stateDirFor("/repo"))
}

func TestDefaultStatePath(t *testing.T) {
	assert.Equal(t,
		filepath.Join("/repo", "test", "e2e-framework", ".e2ectl-state", "kind-nopulumi.state.json"),
		defaultStatePath("/repo", "kind-nopulumi"))
}
