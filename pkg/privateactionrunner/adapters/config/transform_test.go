// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/sets"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

func TestGetBundleInheritedAllowedActions(t *testing.T) {
	tests := []struct {
		name                     string
		actionsAllowlist         map[string]sets.Set[string]
		expectedInheritedActions map[string]sets.Set[string]
	}{
		{
			name: "returns special actions for existing bundle",
			actionsAllowlist: map[string]sets.Set[string]{
				"com.datadoghq.script": sets.New[string]("action1"),
			},
			expectedInheritedActions: map[string]sets.Set[string]{
				"com.datadoghq.script": sets.New[string]("testConnection", "enrichScript"),
			},
		},
		{
			name: "returns special actions for sibling bundles",
			actionsAllowlist: map[string]sets.Set[string]{
				"com.datadoghq.kubernetes.apps": sets.New[string]("action3"),
			},
			expectedInheritedActions: map[string]sets.Set[string]{
				"com.datadoghq.kubernetes.core": sets.New[string]("testConnection"),
			},
		},
		{
			name: "returns empty when bundle not in allowlist",
			actionsAllowlist: map[string]sets.Set[string]{
				"com.other.bundle": sets.New[string]("action1"),
			},
			expectedInheritedActions: map[string]sets.Set[string]{},
		},
		{
			name: "returns empty when bundle has empty set",
			actionsAllowlist: map[string]sets.Set[string]{
				"com.datadoghq.script": sets.New[string](),
			},
			expectedInheritedActions: map[string]sets.Set[string]{},
		},
		{
			name:                     "returns empty for empty allowlist",
			actionsAllowlist:         map[string]sets.Set[string]{},
			expectedInheritedActions: map[string]sets.Set[string]{},
		},
		{
			name: "returns special actions for multiple matching bundles",
			actionsAllowlist: map[string]sets.Set[string]{
				"com.other.bundle":              sets.New[string]("otherAction"),
				"com.datadoghq.script":          sets.New[string]("action1"),
				"com.datadoghq.gitlab.users":    sets.New[string]("action2"),
				"com.datadoghq.kubernetes.core": sets.New[string]("action3"),
				"com.datadoghq.kubernetes.apps": sets.New[string]("action4"),
				"com.datadoghq.remoteaction":    sets.New[string]("action5"),
			},
			expectedInheritedActions: map[string]sets.Set[string]{
				"com.datadoghq.script":          sets.New[string]("testConnection", "enrichScript"),
				"com.datadoghq.gitlab.users":    sets.New[string]("testConnection"),
				"com.datadoghq.kubernetes.core": sets.New[string]("testConnection"),
				"com.datadoghq.remoteaction":    sets.New[string]("testConnection"),
			},
		},
		{
			name: "returns empty for similar looking bundle	",
			actionsAllowlist: map[string]sets.Set[string]{
				"com.datadoghq.dd":           sets.New[string]("action1"),
				"com.datadoghq.dd.subbundle": sets.New[string]("action2"),
			},
			expectedInheritedActions: map[string]sets.Set[string]{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetBundleInheritedAllowedActions(tt.actionsAllowlist)
			assert.Equal(t, tt.expectedInheritedActions, result)
		})
	}
}

func TestGetDatadogHost(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		expected string
	}{
		{
			name:     "removes https prefix and trailing dot",
			endpoint: "https://api.datadoghq.com.",
			expected: "api.datadoghq.com",
		},
		{
			name:     "removes https prefix only",
			endpoint: "https://api.datadoghq.com",
			expected: "api.datadoghq.com",
		},
		{
			name:     "removes trailing dot only",
			endpoint: "api.datadoghq.com.",
			expected: "api.datadoghq.com",
		},
		{
			name:     "handles clean URL without modifications",
			endpoint: "api.datadoghq.com",
			expected: "api.datadoghq.com",
		},
		{
			name:     "handles empty string",
			endpoint: "",
			expected: "",
		},
		{
			name:     "handles EU site with https and trailing dot",
			endpoint: "https://api.datadoghq.eu.",
			expected: "api.datadoghq.eu",
		},
		{
			name:     "handles gov site",
			endpoint: "https://api.ddog-gov.com.",
			expected: "api.ddog-gov.com",
		},
		{
			name:     "handles custom domain",
			endpoint: "https://custom.endpoint.example.com.",
			expected: "custom.endpoint.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getDatadogHost(tt.endpoint)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFromDDConfig(t *testing.T) {
	tests := []struct {
		name           string
		site           string
		ddURL          string
		expectedDDHost string
		expectedDDSite string
	}{
		{
			name:           "US site (datadoghq.com)",
			site:           "datadoghq.com",
			ddURL:          "",
			expectedDDHost: "api.datadoghq.com",
			expectedDDSite: "datadoghq.com",
		},
		{
			name:           "EU site (datadoghq.eu)",
			site:           "datadoghq.eu",
			ddURL:          "",
			expectedDDHost: "api.datadoghq.eu",
			expectedDDSite: "datadoghq.eu",
		},
		{
			name:           "Gov site (ddog-gov.com)",
			site:           "ddog-gov.com",
			ddURL:          "",
			expectedDDHost: "api.ddog-gov.com",
			expectedDDSite: "ddog-gov.com",
		},
		{
			name:           "dd_url overrides site",
			site:           "datadoghq.com",
			ddURL:          "https://api.datadoghq.eu.",
			expectedDDHost: "api.datadoghq.eu",
			expectedDDSite: "datadoghq.eu",
		},
		{
			name:           "custom domain via dd_url",
			site:           "",
			ddURL:          "https://custom.endpoint.example.com.",
			expectedDDHost: "custom.endpoint.example.com",
			expectedDDSite: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := configmock.New(t)

			// Set required configuration values
			if tt.site != "" {
				mockConfig.SetWithoutSource("site", tt.site)
			}
			if tt.ddURL != "" {
				mockConfig.SetWithoutSource("dd_url", tt.ddURL)
			}

			// Set minimal required PAR config to avoid errors
			mockConfig.SetWithoutSource(setup.PARPrivateKey, "")
			mockConfig.SetWithoutSource(setup.PARUrn, "")

			// Call FromDDConfig
			cfg, err := FromDDConfig(mockConfig)
			require.NoError(t, err)

			// Verify both DDHost and DatadogSite are set correctly
			assert.Equal(t, tt.expectedDDHost, cfg.DDHost, "DDHost mismatch")
			assert.Equal(t, tt.expectedDDSite, cfg.DatadogSite, "DatadogSite mismatch")

			// Verify DDApiHost is derived from site, not from dd_url
			assert.Equal(t, "api."+tt.expectedDDSite, cfg.DDApiHost, "DDApiHost should be api.<site>")
		})
	}
}

func TestMakeActionsAllowlistDefaultActionsEnabled(t *testing.T) {
	t.Run("cluster agent default actions are included when default_actions_enabled is true", func(t *testing.T) {
		flavor.SetFlavor(flavor.ClusterAgent)
		defer flavor.SetFlavor(flavor.DefaultAgent)

		mockConfig := configmock.New(t)
		mockConfig.SetWithoutSource(setup.PARActionsAllowlist, []string{})
		mockConfig.SetWithoutSource(setup.PARDefaultActionsEnabled, true)

		allowlist := makeActionsAllowlist(mockConfig)

		assert.True(t, allowlist["com.datadoghq.kubernetes.apps"].Has("listDeployment"))
		assert.True(t, allowlist["com.datadoghq.kubernetes.core"].Has("getPod"))
		assert.True(t, allowlist["com.datadoghq.kubernetes.batch"].Has("getJob"))
		// common actions should also be present
		assert.True(t, allowlist["com.datadoghq.remoteaction.networks"].Has("runNetworkPath"))
		assert.True(t, allowlist["com.datadoghq.remoteaction.rshell"].Has("runCommand"))
		// inherited actions should also be present for the kubernetes prefix
		assert.True(t, allowlist["com.datadoghq.kubernetes.core"].Has("testConnection"))
	})

	t.Run("non-cluster-agent flavor returns common default actions only", func(t *testing.T) {
		flavor.SetFlavor(flavor.DefaultAgent)

		mockConfig := configmock.New(t)
		mockConfig.SetWithoutSource(setup.PARActionsAllowlist, []string{})
		mockConfig.SetWithoutSource(setup.PARDefaultActionsEnabled, true)

		allowlist := makeActionsAllowlist(mockConfig)

		// common actions should be present
		assert.True(t, allowlist["com.datadoghq.remoteaction.networks"].Has("runNetworkPath"))
		assert.True(t, allowlist["com.datadoghq.remoteaction.rshell"].Has("runCommand"))
		// cluster-agent-specific actions should NOT be present
		_, hasK8sApps := allowlist["com.datadoghq.kubernetes.apps"]
		assert.False(t, hasK8sApps)
	})

	t.Run("default actions are excluded when default_actions_enabled is false", func(t *testing.T) {
		flavor.SetFlavor(flavor.ClusterAgent)
		defer flavor.SetFlavor(flavor.DefaultAgent)

		mockConfig := configmock.New(t)
		mockConfig.SetWithoutSource(setup.PARActionsAllowlist, []string{})
		mockConfig.SetWithoutSource(setup.PARDefaultActionsEnabled, false)

		allowlist := makeActionsAllowlist(mockConfig)

		assert.Empty(t, allowlist)
	})

	t.Run("cluster agent default actions merge with explicit allowlist", func(t *testing.T) {
		flavor.SetFlavor(flavor.ClusterAgent)
		defer flavor.SetFlavor(flavor.DefaultAgent)

		mockConfig := configmock.New(t)
		mockConfig.SetWithoutSource(setup.PARActionsAllowlist, []string{"com.datadoghq.http.sendRequest"})
		mockConfig.SetWithoutSource(setup.PARDefaultActionsEnabled, true)

		allowlist := makeActionsAllowlist(mockConfig)

		assert.True(t, allowlist["com.datadoghq.kubernetes.apps"].Has("listDeployment"))
		assert.True(t, allowlist["com.datadoghq.http"].Has("sendRequest"))
	})

	t.Run("explicit allowlist works without default actions", func(t *testing.T) {
		mockConfig := configmock.New(t)
		mockConfig.SetWithoutSource(setup.PARActionsAllowlist, []string{"com.datadoghq.http.sendRequest"})
		mockConfig.SetWithoutSource(setup.PARDefaultActionsEnabled, false)

		allowlist := makeActionsAllowlist(mockConfig)

		assert.True(t, allowlist["com.datadoghq.http"].Has("sendRequest"))
		_, hasK8sApps := allowlist["com.datadoghq.kubernetes.apps"]
		assert.False(t, hasK8sApps)
	})
}

func TestFromDDConfigPARRestrictedShellAllowedPathsUnset(t *testing.T) {
	// Unset key: the registered default is ["/"], a sentinel that admits
	// every backend-allowed path through containment matching. The
	// transform returns it verbatim.
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource(setup.PARPrivateKey, "")
	mockConfig.SetWithoutSource(setup.PARUrn, "")

	cfg, err := FromDDConfig(mockConfig)
	require.NoError(t, err)
	assert.Equal(t, []string{"/"}, cfg.RShellAllowedPaths)
}

func TestFromDDConfigPARRestrictedShellAllowedPathsSet(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource(setup.PARPrivateKey, "")
	mockConfig.SetWithoutSource(setup.PARUrn, "")
	mockConfig.SetWithoutSource(setup.PARRestrictedShellAllowedPaths, []string{"/var/log", "/tmp"})

	cfg, err := FromDDConfig(mockConfig)
	require.NoError(t, err)
	assert.Equal(t, []string{"/var/log", "/tmp"}, cfg.RShellAllowedPaths)
}

func TestFromDDConfigPARRestrictedShellAllowedPathsEmpty(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource(setup.PARPrivateKey, "")
	mockConfig.SetWithoutSource(setup.PARUrn, "")
	mockConfig.SetWithoutSource(setup.PARRestrictedShellAllowedPaths, []string{})

	cfg, err := FromDDConfig(mockConfig)
	require.NoError(t, err)
	// Explicit empty: operator opts in to blocking everything.
	assert.NotNil(t, cfg.RShellAllowedPaths)
	assert.Empty(t, cfg.RShellAllowedPaths)
}

func TestFromDDConfigPARRestrictedShellAllowedCommandsUnset(t *testing.T) {
	// Unset key: the registered default is ["rshell:*"], the wildcard
	// sentinel that admits every backend command in the rshell namespace.
	// The transform returns it verbatim.
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource(setup.PARPrivateKey, "")
	mockConfig.SetWithoutSource(setup.PARUrn, "")

	cfg, err := FromDDConfig(mockConfig)
	require.NoError(t, err)
	assert.Equal(t, []string{"rshell:*"}, cfg.RShellAllowedCommands)
}

func TestFromDDConfigPARRestrictedShellAllowedCommandsSet(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource(setup.PARPrivateKey, "")
	mockConfig.SetWithoutSource(setup.PARUrn, "")
	mockConfig.SetWithoutSource(setup.PARRestrictedShellAllowedCommands, []string{"cat", "ls"})

	cfg, err := FromDDConfig(mockConfig)
	require.NoError(t, err)
	assert.Equal(t, []string{"cat", "ls"}, cfg.RShellAllowedCommands)
}

func TestFromDDConfigPARRestrictedShellAllowedCommandsEmpty(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource(setup.PARPrivateKey, "")
	mockConfig.SetWithoutSource(setup.PARUrn, "")
	mockConfig.SetWithoutSource(setup.PARRestrictedShellAllowedCommands, []string{})

	cfg, err := FromDDConfig(mockConfig)
	require.NoError(t, err)
	// Explicit empty list: operator opts in to blocking every command.
	// Distinct from the unset case above.
	assert.NotNil(t, cfg.RShellAllowedCommands)
	assert.Empty(t, cfg.RShellAllowedCommands)
}

// TestFromDDConfigPARRestrictedShellAllowedPathsEmptyYAML pins the
// kill-switch contract for `allowed_paths: []`: GetStringSlice returns a
// nil slice for the explicit YAML empty list, and the transform forwards
// that as-is. The handler's downstream dedup pass turns nil into a non-nil
// empty slice, which produces an empty intersection — the kill-switch.
// The slice value here is "no entries" regardless of nil/non-nil shape.
func TestFromDDConfigPARRestrictedShellAllowedPathsEmptyYAML(t *testing.T) {
	yaml := `
private_action_runner:
  restricted_shell:
    allowed_paths: []
`
	mockConfig := configmock.NewFromYAML(t, yaml)

	cfg, err := FromDDConfig(mockConfig)
	require.NoError(t, err)
	assert.Empty(t, cfg.RShellAllowedPaths, "YAML [] must surface as an empty slice; kill-switch is enforced by the handler intersection on this input")
}

func TestFromDDConfigPARRestrictedShellAllowedCommandsEmptyYAML(t *testing.T) {
	yaml := `
private_action_runner:
  restricted_shell:
    allowed_commands: []
`
	mockConfig := configmock.NewFromYAML(t, yaml)

	cfg, err := FromDDConfig(mockConfig)
	require.NoError(t, err)
	assert.Empty(t, cfg.RShellAllowedCommands, "YAML [] must surface as an empty slice; kill-switch is enforced by the handler intersection on this input")
}

func TestFromDDConfigPARRestrictedShellAllowedPathsPassesThroughFileEntries(t *testing.T) {
	// File entries are warned about at load time but not dropped — the
	// intersection layer and rshell's own sandbox filter them. The
	// transform's job is to surface the misconfiguration; it does not
	// rewrite the operator's written list.
	tmpDir := t.TempDir()
	fp := filepath.Join(tmpDir, "file.txt")
	require.NoError(t, os.WriteFile(fp, []byte("x"), 0o600))

	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource(setup.PARPrivateKey, "")
	mockConfig.SetWithoutSource(setup.PARUrn, "")
	mockConfig.SetWithoutSource(setup.PARRestrictedShellAllowedPaths, []string{tmpDir, fp})

	cfg, err := FromDDConfig(mockConfig)
	require.NoError(t, err)
	assert.Equal(t, []string{tmpDir, fp}, cfg.RShellAllowedPaths)
}

func TestFromDDConfigPARRestrictedShellAllowedPathsPassesThroughBackslash(t *testing.T) {
	// Backslash-containing entries are preserved in the returned slice so
	// the handler still sees what the operator wrote; the transform also
	// logs a warning so a Windows-native path configured by mistake does
	// not silently produce an empty intersection without feedback.
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource(setup.PARPrivateKey, "")
	mockConfig.SetWithoutSource(setup.PARUrn, "")
	mockConfig.SetWithoutSource(setup.PARRestrictedShellAllowedPaths, []string{`C:\Data`, "/var/log"})

	cfg, err := FromDDConfig(mockConfig)
	require.NoError(t, err)
	assert.Equal(t, []string{`C:\Data`, "/var/log"}, cfg.RShellAllowedPaths)
}

func TestFromDDConfigPARRestrictedShellAllowedCommandsPassesThroughUnnamespaced(t *testing.T) {
	// Unnamespaced entries are preserved in the returned slice so the
	// intersection layer can surface them (as silent no-matches). The
	// transform also emits a log warning about them, which is not asserted
	// here — the point of this test is that unnamespaced entries do not
	// cause config load to fail.
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource(setup.PARPrivateKey, "")
	mockConfig.SetWithoutSource(setup.PARUrn, "")
	mockConfig.SetWithoutSource(setup.PARRestrictedShellAllowedCommands, []string{"cat", "rshell:ls"})

	cfg, err := FromDDConfig(mockConfig)
	require.NoError(t, err)
	assert.Equal(t, []string{"cat", "rshell:ls"}, cfg.RShellAllowedCommands)
}

func TestFromDDConfigPARRestrictedShellAllowedAbsentYAML(t *testing.T) {
	// No restricted_shell block at all: both axes fall back to their
	// registered sentinels — ["/"] for paths, ["rshell:*"] for commands —
	// which the operator-side intersection treats as "allow whatever the
	// backend allowed".
	yaml := `
private_action_runner:
  enabled: true
`
	mockConfig := configmock.NewFromYAML(t, yaml)

	cfg, err := FromDDConfig(mockConfig)
	require.NoError(t, err)
	assert.Equal(t, []string{"/"}, cfg.RShellAllowedPaths)
	assert.Equal(t, []string{"rshell:*"}, cfg.RShellAllowedCommands)
}
