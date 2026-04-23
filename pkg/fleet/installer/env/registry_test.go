// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package env

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeCfg is a minimal ConfigReader for testing BuildRegistryFromConfigAndEnv
// without pulling in the full fx config.Component.
type fakeCfg struct {
	strings    map[string]string
	stringMaps map[string]map[string]interface{}
}

func (f fakeCfg) GetString(key string) string { return f.strings[key] }
func (f fakeCfg) GetStringMap(key string) map[string]interface{} {
	return f.stringMaps[key]
}

func TestPackageNameFromURL(t *testing.T) {
	cases := map[string]string{
		"oci://install.datadoghq.com/agent-package:1.2.3":           "datadog-agent",
		"oci://install.datad0g.com/agent-package:pipeline-123":      "datadog-agent",
		"oci://install.datadoghq.com/apm-inject-package:1.0":        "datadog-apm-inject",
		"oci://install.datadoghq.com/installer-package@sha256:deef": "datadog-installer",
		"oci://install.datadoghq.com/agent-package":                 "datadog-agent",
		"oci://install.datadoghq.com/malformed:1.2.3":               "", // no -package suffix
		"":             "",
		"-package:1.2": "", // empty slug
	}
	for url, want := range cases {
		assert.Equal(t, want, PackageNameFromURL(url), "url=%q", url)
	}
}

func TestResolveFallbackChain(t *testing.T) {
	r := RegistryConfig{
		Default: RegistryEntry{URL: "default.io", Auth: "gcr", Username: "du", Password: "dp"},
		Packages: map[string]PackageRegistry{
			"datadog-agent": {
				RegistryEntry: RegistryEntry{URL: "agent.io"},
				Extensions: map[string]RegistryEntry{
					"ddot": {Auth: "password", Username: "eu"},
				},
			},
		},
	}

	// Unknown package → default only.
	got := r.Resolve("datadog-unknown", "")
	assert.Equal(t, RegistryEntry{URL: "default.io", Auth: "gcr", Username: "du", Password: "dp"}, got)

	// Known package, no ext → package URL overrides default URL;
	// auth/username/password fall through to default.
	got = r.Resolve("datadog-agent", "")
	assert.Equal(t, RegistryEntry{URL: "agent.io", Auth: "gcr", Username: "du", Password: "dp"}, got)

	// Known package + ext → ext auth+username override;
	// URL falls through to package; password falls through to default.
	got = r.Resolve("datadog-agent", "ddot")
	assert.Equal(t, RegistryEntry{URL: "agent.io", Auth: "password", Username: "eu", Password: "dp"}, got)

	// Empty pkg → default.
	got = r.Resolve("", "")
	assert.Equal(t, RegistryEntry{URL: "default.io", Auth: "gcr", Username: "du", Password: "dp"}, got)
}

func TestRegistryConfigJSONRoundTrip(t *testing.T) {
	in := RegistryConfig{
		Default: RegistryEntry{URL: "default.io", Auth: "gcr"},
		Packages: map[string]PackageRegistry{
			"datadog-agent": {
				RegistryEntry: RegistryEntry{URL: "agent.io"},
				Extensions: map[string]RegistryEntry{
					"ddot": {Auth: "password", Username: "u", Password: "p"},
				},
			},
			"datadog-apm-inject": {RegistryEntry: RegistryEntry{URL: "inject.io"}},
		},
	}

	blob, err := json.Marshal(in)
	require.NoError(t, err)

	// Standard nested layout: top-level object with "default" + "packages".
	assert.Contains(t, string(blob), `"default":`)
	assert.Contains(t, string(blob), `"packages":`)
	assert.Contains(t, string(blob), `"datadog-agent":`)
	assert.Contains(t, string(blob), `"datadog-apm-inject":`)

	var out RegistryConfig
	require.NoError(t, json.Unmarshal(blob, &out))
	assert.Equal(t, in, out)
}

func TestRegistryConfigUnmarshalEmpty(t *testing.T) {
	var out RegistryConfig
	require.NoError(t, json.Unmarshal([]byte(`{}`), &out))
	assert.Equal(t, RegistryConfig{}, out)
}

func TestRegistryConfigUnmarshalDefaultOnly(t *testing.T) {
	var out RegistryConfig
	require.NoError(t, json.Unmarshal([]byte(`{"default":{"url":"x.io"}}`), &out))
	assert.Equal(t, RegistryEntry{URL: "x.io"}, out.Default)
	assert.Empty(t, out.Packages)
}

func TestRegistryConfigMarshalEmpty(t *testing.T) {
	// Empty Registry marshals to {"default":{}} (no "packages" since
	// the map is nil and the field is omitempty).
	blob, err := json.Marshal(RegistryConfig{})
	require.NoError(t, err)
	assert.Equal(t, `{"default":{}}`, string(blob))
}

func TestBuildRegistryFromConfigAndEnv(t *testing.T) {
	t.Run("yaml only", func(t *testing.T) {
		clearRegistryEnv(t)
		cfg := fakeCfg{
			strings: map[string]string{
				"installer.registry.url":      "yaml.io",
				"installer.registry.auth":     "gcr",
				"installer.registry.username": "yu",
				"installer.registry.password": "yp",
			},
			stringMaps: map[string]map[string]interface{}{
				"installer.registry.extensions": {
					"datadog-agent": map[string]interface{}{
						"ddot": map[string]interface{}{
							"url":      "ext.io",
							"auth":     "password",
							"username": "eu",
							"password": "ep",
						},
					},
				},
			},
		}
		got, blob, err := BuildRegistryFromConfigAndEnv(cfg)
		require.NoError(t, err)
		assert.Equal(t, RegistryEntry{URL: "yaml.io", Auth: "gcr", Username: "yu", Password: "yp"}, got.Default)
		require.Contains(t, got.Packages, "datadog-agent")
		assert.Equal(t, RegistryEntry{URL: "ext.io", Auth: "password", Username: "eu", Password: "ep"},
			got.Packages["datadog-agent"].Extensions["ddot"])
		assert.NotEmpty(t, blob)
	})

	t.Run("legacy env vars override yaml defaults", func(t *testing.T) {
		clearRegistryEnv(t)
		t.Setenv("DD_INSTALLER_REGISTRY_URL", "env.io")
		t.Setenv("DD_INSTALLER_REGISTRY_AUTH", "password")
		cfg := fakeCfg{
			strings: map[string]string{
				"installer.registry.url":  "yaml.io",
				"installer.registry.auth": "gcr",
			},
		}
		got, _, err := BuildRegistryFromConfigAndEnv(cfg)
		require.NoError(t, err)
		assert.Equal(t, "env.io", got.Default.URL)
		assert.Equal(t, "password", got.Default.Auth)
	})

	t.Run("per-package legacy env var populates Packages slot", func(t *testing.T) {
		clearRegistryEnv(t)
		t.Setenv("DD_INSTALLER_REGISTRY_URL_AGENT_PACKAGE", "mirror.io/agent")
		t.Setenv("DD_INSTALLER_REGISTRY_AUTH_AGENT_PACKAGE", "password")
		cfg := fakeCfg{}
		got, _, err := BuildRegistryFromConfigAndEnv(cfg)
		require.NoError(t, err)
		require.Contains(t, got.Packages, "datadog-agent")
		entry := got.Packages["datadog-agent"]
		assert.Equal(t, "mirror.io/agent", entry.URL)
		assert.Equal(t, "password", entry.Auth)
	})

	t.Run("DD_INSTALLER_REGISTRY JSON wins over legacy + yaml", func(t *testing.T) {
		clearRegistryEnv(t)
		t.Setenv("DD_INSTALLER_REGISTRY_URL", "legacy.io")
		t.Setenv(EnvInstallerRegistry, `{"default":{"url":"canonical.io"}}`)
		cfg := fakeCfg{
			strings: map[string]string{"installer.registry.url": "yaml.io"},
		}
		got, _, err := BuildRegistryFromConfigAndEnv(cfg)
		require.NoError(t, err)
		assert.Equal(t, "canonical.io", got.Default.URL)
	})

	t.Run("legacy per-package merges with yaml extensions on same pkg", func(t *testing.T) {
		clearRegistryEnv(t)
		t.Setenv("DD_INSTALLER_REGISTRY_URL_AGENT_PACKAGE", "from-env.io/agent")
		cfg := fakeCfg{
			stringMaps: map[string]map[string]interface{}{
				"installer.registry.extensions": {
					"datadog-agent": map[string]interface{}{
						"ddot": map[string]interface{}{"url": "ext.io"},
					},
				},
			},
		}
		got, _, err := BuildRegistryFromConfigAndEnv(cfg)
		require.NoError(t, err)
		entry := got.Packages["datadog-agent"]
		assert.Equal(t, "from-env.io/agent", entry.URL)
		assert.Equal(t, RegistryEntry{URL: "ext.io"}, entry.Extensions["ddot"])
	})
}

// clearRegistryEnv unsets any DD_INSTALLER_REGISTRY* vars the process may
// have inherited; t.Cleanup restores them via the saved value.
func clearRegistryEnv(t *testing.T) {
	t.Helper()
	for _, kv := range os.Environ() {
		eq := strings.IndexByte(kv, '=')
		if eq < 0 {
			continue
		}
		key := kv[:eq]
		if key == EnvInstallerRegistry || strings.HasPrefix(key, "DD_INSTALLER_REGISTRY_") {
			original := os.Getenv(key)
			os.Unsetenv(key)
			t.Cleanup(func() { os.Setenv(key, original) })
		}
	}
}
