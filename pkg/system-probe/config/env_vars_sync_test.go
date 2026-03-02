// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package config

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
)

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

func (c *testConfig) Set(key string, value interface{}, source model.Source) {
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

func resolveEnvVar(key string, cfg pkgconfigmodel.Reader) string {
	// Try the auto-generated name: DD_ + UPPER(key with . replaced by _)
	auto := "DD_" + strings.ToUpper(strings.NewReplacer(".", "_").Replace(key))
	os.Setenv(auto, "resolution_sentinel")
	defer os.Unsetenv(auto)
	if cfg.GetString(key) == "resolution_sentinel" {
		return auto
	}
	os.Unsetenv(auto)

	// Fallback: search all registered env vars for one that maps to this key
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

func TestEnableModulesVars(t *testing.T) {
	var c types.Config

	realCoreCfg := mock.New(t)
	realSpCfg := mock.NewSystemProbe(t)

	coreCfgCollector := newTestConfig(t, realCoreCfg)
	spCfgCollector := newTestConfig(t, realSpCfg)

	enableModules(&c, coreCfgCollector, spCfgCollector)

	var envVars []string
	envVars = append(envVars, coreCfgCollector.resolveEnvVars()...)
	envVars = append(envVars, spCfgCollector.resolveEnvVars()...)
	sort.Strings(envVars)

	discoveredList := envVars

	t.Logf("Discovered %d env vars that trigger non-discovery modules:", len(discoveredList))
	for _, v := range discoveredList {
		t.Logf("  %s", v)
	}

	txtPath := filepath.Join("..", "..", "discovery", "module", "testdata", "non_discovery_env_vars.txt")
	txtBytes, err := os.ReadFile(txtPath)
	require.NoErrorf(t, err,
		"Cannot read canonical list at %s.\n"+
			"Create it with the discovered env vars listed above.", txtPath)

	var canonicalSorted []string
	for _, line := range strings.Split(strings.TrimSpace(string(txtBytes)), "\n") {
		if line != "" {
			canonicalSorted = append(canonicalSorted, line)
		}
	}
	sort.Strings(canonicalSorted)

	assert.Equal(t, canonicalSorted, discoveredList,
		"Mismatch between discovered env vars and canonical list at %s.\n"+
			"Update the file with the discovered list shown above.\n"+
			"Then update NON_DISCOVERY_ENV_VARS in pkg/discovery/module/rust/src/config.rs.",
		txtPath)
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
