// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fxconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoadAndExportEnv_endToEnd writes a temp datadog.yaml, clears every
// DD_* env var, runs LoadAndExportEnv, and checks that the yaml fields are
// translated to the expected env vars.
func TestLoadAndExportEnv_endToEnd(t *testing.T) {
	dir := t.TempDir()
	yaml := `
api_key: yaml-api-key-0123456789abcdef01
site: datadoghq.eu
hostname: yaml-host
remote_updates: true
installer:
  mirror: https://mirror.example.com
  registry:
    url: yaml-registry.example.com
    auth: yaml-auth
    username: yaml-user
    password: yaml-pass
    extensions:
      datadog-agent:
        ddot:
          url: custom.registry.com
          auth: ddot-auth
          username: ddot-user
          password: ddot-pass
        other-ext:
          url: other.registry.com
proxy:
  http: http://proxy.example.com:8080
  https: http://proxy.example.com:8443
  no_proxy:
    - localhost
    - 127.0.0.1
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "datadog.yaml"), []byte(yaml), 0o644))

	// Clean slate for the env vars we care about.
	clearKeys := []string{
		"DD_API_KEY", "DD_SITE", "DD_HOSTNAME", "DD_REMOTE_UPDATES",
		"DD_INSTALLER_MIRROR",
		"DD_INSTALLER_REGISTRY_URL", "DD_INSTALLER_REGISTRY_AUTH",
		"DD_INSTALLER_REGISTRY_USERNAME", "DD_INSTALLER_REGISTRY_PASSWORD",
		"DD_PROXY_HTTP", "DD_PROXY_HTTPS", "DD_PROXY_NO_PROXY",
		"DD_INSTALLER_REGISTRY_EXT_URL_DATADOG_AGENT__DDOT",
		"DD_INSTALLER_REGISTRY_EXT_AUTH_DATADOG_AGENT__DDOT",
		"DD_INSTALLER_REGISTRY_EXT_USERNAME_DATADOG_AGENT__DDOT",
		"DD_INSTALLER_REGISTRY_EXT_PASSWORD_DATADOG_AGENT__DDOT",
		"DD_INSTALLER_REGISTRY_EXT_URL_DATADOG_AGENT__OTHER_EXT",
	}
	for _, k := range clearKeys {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}

	LoadAndExportEnv(dir)

	expected := map[string]string{
		"DD_API_KEY":                     "yaml-api-key-0123456789abcdef01",
		"DD_SITE":                        "datadoghq.eu",
		"DD_HOSTNAME":                    "yaml-host",
		"DD_REMOTE_UPDATES":              "true",
		"DD_INSTALLER_MIRROR":            "https://mirror.example.com",
		"DD_INSTALLER_REGISTRY_URL":      "yaml-registry.example.com",
		"DD_INSTALLER_REGISTRY_AUTH":     "yaml-auth",
		"DD_INSTALLER_REGISTRY_USERNAME": "yaml-user",
		"DD_INSTALLER_REGISTRY_PASSWORD": "yaml-pass",
		"DD_PROXY_HTTP":                  "http://proxy.example.com:8080",
		"DD_PROXY_HTTPS":                 "http://proxy.example.com:8443",
		"DD_INSTALLER_REGISTRY_EXT_URL_DATADOG_AGENT__DDOT":      "custom.registry.com",
		"DD_INSTALLER_REGISTRY_EXT_AUTH_DATADOG_AGENT__DDOT":     "ddot-auth",
		"DD_INSTALLER_REGISTRY_EXT_USERNAME_DATADOG_AGENT__DDOT": "ddot-user",
		"DD_INSTALLER_REGISTRY_EXT_PASSWORD_DATADOG_AGENT__DDOT": "ddot-pass",
		"DD_INSTALLER_REGISTRY_EXT_URL_DATADOG_AGENT__OTHER_EXT": "other.registry.com",
	}
	for k, want := range expected {
		assert.Equal(t, want, os.Getenv(k), "env var %s", k)
	}
	// DD_PROXY_NO_PROXY may include default agent-config no_proxy entries
	// (cloud metadata endpoints) that the agent adds at load time; assert
	// the yaml-provided entries are present rather than equality.
	noProxy := os.Getenv("DD_PROXY_NO_PROXY")
	assert.Contains(t, noProxy, "localhost", "DD_PROXY_NO_PROXY should contain localhost")
	assert.Contains(t, noProxy, "127.0.0.1", "DD_PROXY_NO_PROXY should contain 127.0.0.1")
}

// TestLoadAndExportEnv_respectsPreSetEnv asserts that when a DD_* var is
// already set, the bootstrap does not overwrite it.
func TestLoadAndExportEnv_respectsPreSetEnv(t *testing.T) {
	dir := t.TempDir()
	yaml := "api_key: yaml-api-key-0123456789abcdef01\nsite: datadoghq.eu\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "datadog.yaml"), []byte(yaml), 0o644))

	t.Setenv("DD_SITE", "explicit.example.com")
	os.Unsetenv("DD_API_KEY")

	LoadAndExportEnv(dir)

	assert.Equal(t, "explicit.example.com", os.Getenv("DD_SITE"),
		"pre-set DD_SITE must be preserved; bootstrap must not overwrite")
	assert.Equal(t, "yaml-api-key-0123456789abcdef01", os.Getenv("DD_API_KEY"),
		"DD_API_KEY sourced from yaml since it was unset")
}

// TestLoadAndExportEnv_missingYAML asserts that a missing file is a
// non-fatal best-effort: the bootstrap does not panic and returns cleanly.
// We don't assert absence of env vars because the agent config layer may
// fall back to an auto-discovered file (e.g. /etc/datadog-agent/datadog.yaml
// on a host that has the agent installed).
func TestLoadAndExportEnv_missingYAML(t *testing.T) {
	dir := t.TempDir() // empty, no datadog.yaml
	assert.NotPanics(t, func() { LoadAndExportEnv(dir) })
}

// TestExportExtensionOverrides_emptyIsNoop covers the defensive path.
func TestExportExtensionOverrides_emptyIsNoop(t *testing.T) {
	before := len(os.Environ())
	exportExtensionOverrides(nil)
	exportExtensionOverrides(map[string]interface{}{})
	assert.Equal(t, before, len(os.Environ()))
}

// TestEnvKey checks the lowercase-dash to uppercase-underscore mapping.
func TestEnvKey(t *testing.T) {
	assert.Equal(t, "DATADOG_AGENT", envKey("datadog-agent"))
	assert.Equal(t, "OTHER_EXT", envKey("other-ext"))
	// A key with no special chars stays upper.
	assert.Equal(t, "DDOT", envKey("ddot"))
	// Sanity: round-trip consistency with the env package's decoder.
	for _, in := range []string{"a", "foo-bar", "x-y-z"} {
		enc := envKey(in)
		dec := strings.ToLower(strings.ReplaceAll(enc, "_", "-"))
		assert.Equal(t, in, dec, "round-trip %q", in)
	}
}
