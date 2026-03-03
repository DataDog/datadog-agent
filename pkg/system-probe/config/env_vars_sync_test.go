// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
)

type canonicalConfig struct {
	EnvVars  []string `json:"env_vars"`
	YAMLKeys []string `json:"yaml_keys"`
}

type testConfig struct {
	model.Config
	t    *testing.T
	keys map[string]struct{}
}

func newTestConfig(t *testing.T, cfg model.Config) *testConfig {
	return &testConfig{
		Config: cfg,
		t:      t,
		keys:   make(map[string]struct{}),
	}
}

func (c *testConfig) GetBool(key string) bool {
	// If any of the keys are default true, we may need special handling to ensure
	// that the correct code paths are exercised.  Ignore this case for for now but
	// make sure we don't silently miss it if it happens later.
	assert.False(c.t, c.Config.GetBool(key), "default true conditions not handled: %s", key)

	if key != "discovery.enabled" {
		c.keys[key] = struct{}{}
	}

	return false
}

func (c *testConfig) Set(_ string, _ interface{}, _ model.Source) {
}

func (c *testConfig) sortedKeys() []string {
	keys := make([]string, 0, len(c.keys))
	for k := range c.keys {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (c *testConfig) resolveEnvVars() []string {
	var envVars []string
	for key := range c.keys {
		envVar := resolveEnvVar(key, c.Config)
		assert.NotEmpty(c.t, envVar, "env var not found for key: %s", key)
		envVars = append(envVars, envVar)
	}
	return envVars
}

func resolveEnvVar(key string, cfg model.Reader) string {
	// Try the auto-generated name: DD_ + UPPER(key with . replaced by _)
	auto := "DD_" + strings.ToUpper(strings.NewReplacer(".", "_").Replace(key))
	os.Setenv(auto, "resolution_sentinel")
	defer os.Unsetenv(auto)
	if cfg.GetString(key) == "resolution_sentinel" {
		return auto
	}
	os.Unsetenv(auto)

	// Fallback: search all registered env vars for one that maps to this key.  The config
	// model doesn't support finding env vars by key, so we have to search all
	// registered env vars.
	for _, ev := range cfg.GetEnvVars() {
		os.Setenv(ev, "resolution_sentinel")
		if cfg.GetString(key) == "resolution_sentinel" {
			os.Unsetenv(ev)
			return ev
		}
		os.Unsetenv(ev)
	}
	return ""
}

// TestEnableModulesVars collects all the env vars and YAML keys that trigger
// non-discovery modules and verifies that they match the canonical JSON file.
func TestEnableModulesVars(t *testing.T) {
	var c types.Config

	realCoreCfg := mock.New(t)
	realSpCfg := mock.NewSystemProbe(t)

	coreCfgCollector := newTestConfig(t, realCoreCfg)
	spCfgCollector := newTestConfig(t, realSpCfg)

	enableModules(&c, coreCfgCollector, spCfgCollector)

	// Collect YAML keys from both config collectors.
	var yamlKeys []string
	yamlKeys = append(yamlKeys, coreCfgCollector.sortedKeys()...)
	yamlKeys = append(yamlKeys, spCfgCollector.sortedKeys()...)
	sort.Strings(yamlKeys)

	// Collect env vars from both config collectors.
	var envVars []string
	envVars = append(envVars, coreCfgCollector.resolveEnvVars()...)
	envVars = append(envVars, spCfgCollector.resolveEnvVars()...)
	sort.Strings(envVars)

	t.Logf("Discovered %d env vars:", len(envVars))
	for _, v := range envVars {
		t.Logf("  %s", v)
	}
	t.Logf("Discovered %d YAML keys:", len(yamlKeys))
	for _, v := range yamlKeys {
		t.Logf("  %s", v)
	}

	jsonPath := filepath.Join("testdata", "non_discovery_module_config.json")
	jsonBytes, err := os.ReadFile(jsonPath)
	require.NoErrorf(t, err,
		"Cannot read canonical JSON at %s.\n"+
			"Create it with the discovered env vars and YAML keys listed above.", jsonPath)

	var canonical canonicalConfig
	require.NoError(t, json.Unmarshal(jsonBytes, &canonical),
		"Failed to parse canonical JSON at %s", jsonPath)

	assert.Equal(t, canonical.EnvVars, envVars,
		"Mismatch between discovered env vars and canonical list at %s.\n"+
			"Update the env_vars array with the discovered list shown above.\n"+
			"Then update NON_DISCOVERY_ENV_VARS in pkg/discovery/module/rust/src/config.rs.",
		jsonPath)

	assert.Equal(t, canonical.YAMLKeys, yamlKeys,
		"Mismatch between discovered YAML keys and canonical list at %s.\n"+
			"Update the yaml_keys array with the discovered list shown above.\n"+
			"Then update NON_DISCOVERY_YAML_KEYS in pkg/discovery/module/rust/src/config.rs.",
		jsonPath)
}

// TestNonDiscoveryEnvVarsDiscoveryOnly verifies that setting only the
// discovery env var does not trigger any non-discovery modules.
func TestNonDiscoveryEnvVarsDiscoveryOnly(t *testing.T) {
	mock.New(t)
	mock.NewSystemProbe(t)

	t.Setenv("DD_DISCOVERY_ENABLED", "true")

	cfg, err := load()
	require.NoError(t, err)

	for mod := range cfg.EnabledModules {
		if mod != DiscoveryModule {
			t.Fatalf("expected only DiscoveryModule, but %s is also enabled", mod)
		}
	}
}
