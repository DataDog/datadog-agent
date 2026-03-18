// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/)
// Copyright 2024-present Datadog, Inc.

//go:build test

package converterimpl

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/confmap"
)

var uuidRE = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// TestGenerateUUID verifies that generateUUID produces a valid UUID v4.
func TestGenerateUUID(t *testing.T) {
	uid, err := generateUUID()
	require.NoError(t, err)
	assert.Regexp(t, uuidRE, uid)

	uid2, err := generateUUID()
	require.NoError(t, err)
	assert.NotEqual(t, uid, uid2, "two generated UUIDs should differ")
}

// TestLoadOrCreateInstanceUID verifies that a new UUID is generated on the first
// call, written to disk, and the same value is returned on subsequent calls.
//
// speky:OTELCOL#T030
func TestLoadOrCreateInstanceUID(t *testing.T) {
	dir := t.TempDir()
	c := &ddConverter{
		coreConfig: config.NewMockFromYAML(t, "run_path: "+dir),
	}

	uid1, err := c.loadOrCreateInstanceUID()
	require.NoError(t, err)
	assert.Regexp(t, uuidRE, uid1)

	// File must have been written.
	data, err := os.ReadFile(filepath.Join(dir, "otel-instance-uid"))
	require.NoError(t, err)
	assert.Equal(t, uid1, string(data))

	// Second call must return the same UID (reads from disk).
	uid2, err := c.loadOrCreateInstanceUID()
	require.NoError(t, err)
	assert.Equal(t, uid1, uid2)
}

// TestEnsureOpampInstanceUID_InjectsUID verifies that when the opamp extension is
// present in the config with no instance_uid, the converter injects one.
//
// speky:OTELCOL#T030
func TestEnsureOpampInstanceUID_InjectsUID(t *testing.T) {
	dir := t.TempDir()
	c := &ddConverter{
		coreConfig: config.NewMockFromYAML(t, "run_path: "+dir),
	}

	conf := confmapFromYAML(t, `
extensions:
  opamp:
    server:
      ws:
        endpoint: ws://localhost:4320/v1/opamp
service:
  extensions: [opamp]
  pipelines: {}
`)

	c.ensureOpampInstanceUID(conf)

	m := conf.ToStringMap()
	ext := m["extensions"].(map[string]any)
	opampCfg := ext["opamp"].(map[string]any)
	uid, ok := opampCfg["instance_uid"].(string)
	require.True(t, ok, "instance_uid should be set")
	assert.Regexp(t, uuidRE, uid)
}

// TestEnsureOpampInstanceUID_PreservesExisting verifies that a user-supplied
// instance_uid is not overwritten by the converter.
//
// speky:OTELCOL#T030
func TestEnsureOpampInstanceUID_PreservesExisting(t *testing.T) {
	const userUID = "12345678-1234-4234-8234-123456789abc"
	dir := t.TempDir()
	c := &ddConverter{
		coreConfig: config.NewMockFromYAML(t, "run_path: "+dir+"\nsite: datadoghq.com"),
	}

	conf := confmapFromYAML(t, `
extensions:
  opamp:
    instance_uid: `+userUID+`
    server:
      ws:
        endpoint: ws://localhost:4320/v1/opamp
service:
  extensions: [opamp]
  pipelines: {}
`)

	c.ensureOpampInstanceUID(conf)

	m := conf.ToStringMap()
	ext := m["extensions"].(map[string]any)
	opampCfg := ext["opamp"].(map[string]any)
	assert.Equal(t, userUID, opampCfg["instance_uid"])

	// AgentDescription must still be enriched even when the user supplied their own UID.
	desc := opampCfg["agent_description"].(map[string]any)
	attrs := desc["non_identifying_attributes"].(map[string]any)
	assert.NotEmpty(t, attrs["datadoghq.com/site"])
	assert.NotEmpty(t, attrs["datadoghq.com/deployment_type"])
}

// TestEnsureOpampInstanceUID_NoOpamp verifies that the converter is a no-op when
// no opamp extension is present in the config.
func TestEnsureOpampInstanceUID_NoOpamp(t *testing.T) {
	dir := t.TempDir()
	c := &ddConverter{
		coreConfig: config.NewMockFromYAML(t, "run_path: "+dir),
	}

	conf := confmapFromYAML(t, `
extensions:
  health_check:
service:
  extensions: [health_check]
  pipelines: {}
`)
	before := conf.ToStringMap()
	c.ensureOpampInstanceUID(conf)
	assert.Equal(t, before, conf.ToStringMap(), "config should be unchanged when opamp is absent")

	// No UID file should have been created.
	_, err := os.Stat(filepath.Join(dir, "otel-instance-uid"))
	assert.True(t, os.IsNotExist(err))
}

// TestEnrichOpampAgentDescription_InjectsSiteAndDeployment verifies that the
// converter populates datadoghq.com/site and datadoghq.com/deployment_type from
// the core agent config when they are not already set by the user.
//
// speky:OTELCOL#T018
func TestEnrichOpampAgentDescription_InjectsSiteAndDeployment(t *testing.T) {
	c := &ddConverter{
		coreConfig: config.NewMockFromYAML(t, "site: datadoghq.eu"),
	}
	cfg := map[string]any{}
	c.enrichOpampAgentDescription(cfg)

	desc := cfg["agent_description"].(map[string]any)
	attrs := desc["non_identifying_attributes"].(map[string]any)
	assert.Equal(t, "datadoghq.eu", attrs["datadoghq.com/site"])
	assert.Equal(t, "daemonset", attrs["datadoghq.com/deployment_type"])
}

// TestEnrichOpampAgentDescription_GatewayMode verifies that gateway mode is
// reflected in the deployment_type attribute.
//
// speky:OTELCOL#T018
func TestEnrichOpampAgentDescription_GatewayMode(t *testing.T) {
	c := &ddConverter{
		coreConfig: config.NewMockFromYAML(t, "site: datadoghq.com\notelcollector:\n  gateway:\n    mode: true"),
	}
	cfg := map[string]any{}
	c.enrichOpampAgentDescription(cfg)

	desc := cfg["agent_description"].(map[string]any)
	attrs := desc["non_identifying_attributes"].(map[string]any)
	assert.Equal(t, "gateway", attrs["datadoghq.com/deployment_type"])
}

// TestEnrichOpampAgentDescription_PreservesUserValues verifies that user-supplied
// non_identifying_attributes are not overwritten.
//
// speky:OTELCOL#T018
func TestEnrichOpampAgentDescription_PreservesUserValues(t *testing.T) {
	c := &ddConverter{
		coreConfig: config.NewMockFromYAML(t, "site: datadoghq.com"),
	}
	cfg := map[string]any{
		"agent_description": map[string]any{
			"non_identifying_attributes": map[string]any{
				"datadoghq.com/site":            "custom-override.com",
				"datadoghq.com/deployment_type": "custom-type",
			},
		},
	}
	c.enrichOpampAgentDescription(cfg)

	desc := cfg["agent_description"].(map[string]any)
	attrs := desc["non_identifying_attributes"].(map[string]any)
	assert.Equal(t, "custom-override.com", attrs["datadoghq.com/site"])
	assert.Equal(t, "custom-type", attrs["datadoghq.com/deployment_type"])
}

// TestConvertOpamp verifies the full converter pipeline for an opamp extension:
// instance_uid is injected (valid UUID v4) and agent_description is enriched with
// site and deployment_type from the core agent config.
//
// speky:OTELCOL#T030 speky:OTELCOL#T018
func TestConvertOpamp(t *testing.T) {
	dir := t.TempDir()
	acfg := config.NewMockFromYAML(t, "run_path: "+dir+"\nsite: datadoghq.eu\notelcollector:\n  converter:\n    features: []")
	converter, err := NewConverterForAgent(Requires{Conf: acfg})
	require.NoError(t, err)

	conf := confmapFromYAML(t, `
extensions:
  opamp:
    server:
      ws:
        endpoint: ws://localhost:4320/v1/opamp
receivers:
  nop:
exporters:
  nop:
service:
  extensions: [opamp]
  pipelines:
    traces:
      receivers: [nop]
      exporters: [nop]
`)
	converter.Convert(context.Background(), conf)

	m := conf.ToStringMap()
	ext := m["extensions"].(map[string]any)
	opampCfg := ext["opamp"].(map[string]any)

	uid, ok := opampCfg["instance_uid"].(string)
	require.True(t, ok, "instance_uid should be set")
	assert.Regexp(t, uuidRE, uid, "instance_uid should be a UUID v4")

	desc := opampCfg["agent_description"].(map[string]any)
	attrs := desc["non_identifying_attributes"].(map[string]any)
	assert.Equal(t, "datadoghq.eu", attrs["datadoghq.com/site"])
	assert.Equal(t, "daemonset", attrs["datadoghq.com/deployment_type"])
}

// confmapFromYAML is a test helper that parses a YAML string into a *confmap.Conf.
func confmapFromYAML(t *testing.T, yaml string) *confmap.Conf {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "*.yaml")
	require.NoError(t, err)
	_, err = f.WriteString(yaml)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	resolver, err := newResolver([]string{"file:" + f.Name()})
	require.NoError(t, err)
	conf, err := resolver.Resolve(context.Background())
	require.NoError(t, err)
	return conf
}
