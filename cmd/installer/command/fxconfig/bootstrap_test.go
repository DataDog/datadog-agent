// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fxconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
)

func clearDDEnvForTest(t *testing.T, extra ...string) {
	t.Helper()
	targets := map[string]bool{
		"DD_API_KEY":             true,
		"DD_SITE":                true,
		"DD_HOSTNAME":            true,
		"DD_INSTALLER_MIRROR":    true,
		"DD_LOG_LEVEL":           true,
		"DD_REMOTE_UPDATES":      true,
		"DD_TAGS":                true,
		"DD_PROXY_HTTP":          true,
		"DD_PROXY_HTTPS":         true,
		"DD_PROXY_NO_PROXY":      true,
		env.EnvInstallerRegistry: true,
	}
	for _, e := range extra {
		targets[e] = true
	}
	for _, kv := range os.Environ() {
		eq := strings.IndexByte(kv, '=')
		if eq < 0 {
			continue
		}
		key := kv[:eq]
		if targets[key] || strings.HasPrefix(key, "DD_INSTALLER_REGISTRY_") {
			t.Setenv(key, "")
			os.Unsetenv(key)
		}
	}
}

func writeYAML(t *testing.T, dir, content string) string {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "datadog.yaml"), []byte(content), 0o644))
	return dir
}

func TestLoadAndExportEnvYAMLOnly(t *testing.T) {
	clearDDEnvForTest(t)
	dir := t.TempDir()
	writeYAML(t, dir, `
api_key: yaml-api-key
site: datadoghq.eu
hostname: myhost
installer:
  mirror: https://mirror.example.com
  registry:
    url: yaml.registry.com
    auth: gcr
    extensions:
      datadog-agent:
        ddot:
          url: ext.registry.com
          auth: password
          username: eu
          password: ep
tags:
  - env:prod
  - team:fleet
proxy:
  http: http://proxy:8080
  https: http://proxy:8443
  no_proxy:
    - localhost
    - .internal
remote_updates: true
`)

	LoadAndExportEnv(dir)

	assert.Equal(t, "yaml-api-key", os.Getenv("DD_API_KEY"))
	assert.Equal(t, "datadoghq.eu", os.Getenv("DD_SITE"))
	assert.Equal(t, "myhost", os.Getenv("DD_HOSTNAME"))
	assert.Equal(t, "https://mirror.example.com", os.Getenv("DD_INSTALLER_MIRROR"))
	assert.Equal(t, "true", os.Getenv("DD_REMOTE_UPDATES"))
	assert.Equal(t, "http://proxy:8080", os.Getenv("DD_PROXY_HTTP"))
	assert.Equal(t, "http://proxy:8443", os.Getenv("DD_PROXY_HTTPS"))
	// no_proxy values from yaml are joined with agent-added defaults
	// (cloud metadata IPs), so just verify our entries are present.
	assert.Contains(t, os.Getenv("DD_PROXY_NO_PROXY"), "localhost")
	assert.Contains(t, os.Getenv("DD_PROXY_NO_PROXY"), ".internal")
	assert.Contains(t, os.Getenv("DD_TAGS"), "env:prod")
	assert.Contains(t, os.Getenv("DD_TAGS"), "team:fleet")

	// Registry: single DD_INSTALLER_REGISTRY JSON blob.
	blob := os.Getenv(env.EnvInstallerRegistry)
	require.NotEmpty(t, blob)
	var r env.RegistryConfig
	require.NoError(t, json.Unmarshal([]byte(blob), &r))
	assert.Equal(t, "yaml.registry.com", r.Default.URL)
	assert.Equal(t, "gcr", r.Default.Auth)
	require.Contains(t, r.Packages, "datadog-agent")
	require.Contains(t, r.Packages["datadog-agent"].Extensions, "ddot")
	ext := r.Packages["datadog-agent"].Extensions["ddot"]
	assert.Equal(t, "ext.registry.com", ext.URL)
	assert.Equal(t, "password", ext.Auth)
	assert.Equal(t, "eu", ext.Username)
	assert.Equal(t, "ep", ext.Password)
}

func TestLoadAndExportEnvLegacyPerPackageVarAbsorbed(t *testing.T) {
	clearDDEnvForTest(t)
	dir := t.TempDir()
	writeYAML(t, dir, "api_key: yaml-key\n")
	t.Setenv("DD_INSTALLER_REGISTRY_URL_AGENT_PACKAGE", "mirror.io/agent")
	t.Setenv("DD_INSTALLER_REGISTRY_AUTH_AGENT_PACKAGE", "password")

	LoadAndExportEnv(dir)

	// Legacy per-package vars are removed after absorption.
	assert.Equal(t, "", os.Getenv("DD_INSTALLER_REGISTRY_URL_AGENT_PACKAGE"))
	assert.Equal(t, "", os.Getenv("DD_INSTALLER_REGISTRY_AUTH_AGENT_PACKAGE"))

	blob := os.Getenv(env.EnvInstallerRegistry)
	require.NotEmpty(t, blob)
	var r env.RegistryConfig
	require.NoError(t, json.Unmarshal([]byte(blob), &r))
	require.Contains(t, r.Packages, "datadog-agent")
	assert.Equal(t, "mirror.io/agent", r.Packages["datadog-agent"].URL)
	assert.Equal(t, "password", r.Packages["datadog-agent"].Auth)
}

func TestLoadAndExportEnvUserSetJSONWinsOverYAML(t *testing.T) {
	clearDDEnvForTest(t)
	dir := t.TempDir()
	writeYAML(t, dir, `
installer:
  registry:
    url: yaml.registry.com
`)
	// User sets DD_INSTALLER_REGISTRY directly → it wins.
	t.Setenv(env.EnvInstallerRegistry, `{"default":{"url":"canonical.io"}}`)

	LoadAndExportEnv(dir)

	blob := os.Getenv(env.EnvInstallerRegistry)
	require.NotEmpty(t, blob)
	var r env.RegistryConfig
	require.NoError(t, json.Unmarshal([]byte(blob), &r))
	assert.Equal(t, "canonical.io", r.Default.URL)
}

func TestLoadAndExportEnvDoesNotOverwriteUserScalarEnvVars(t *testing.T) {
	clearDDEnvForTest(t)
	dir := t.TempDir()
	writeYAML(t, dir, `
api_key: yaml-key
site: datadoghq.eu
`)

	t.Setenv("DD_API_KEY", "user-env-key")
	t.Setenv("DD_SITE", "datadoghq.com")

	LoadAndExportEnv(dir)

	assert.Equal(t, "user-env-key", os.Getenv("DD_API_KEY"))
	assert.Equal(t, "datadoghq.com", os.Getenv("DD_SITE"))
}

func TestLoadAndExportEnvSkipsWhenFromDaemon(t *testing.T) {
	clearDDEnvForTest(t)
	dir := t.TempDir()
	writeYAML(t, dir, `
api_key: yaml-key
installer:
  registry:
    url: yaml.registry.com
`)

	t.Setenv("DD_INSTALLER_FROM_DAEMON", "true")

	LoadAndExportEnv(dir)

	// Nothing from yaml should have been exported.
	assert.Equal(t, "", os.Getenv("DD_API_KEY"))
	assert.Equal(t, "", os.Getenv(env.EnvInstallerRegistry))
}

func TestLoadAndExportEnvMissingYAMLIsBestEffort(t *testing.T) {
	clearDDEnvForTest(t)
	dir := t.TempDir()
	assert.NotPanics(t, func() { LoadAndExportEnv(dir) })
	assert.Equal(t, "", os.Getenv(env.EnvInstallerRegistry))
}
