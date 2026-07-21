// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package config

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	statsdcomp "github.com/DataDog/datadog-agent/comp/dogstatsd/statsd/def"
	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/sets"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
				"com.datadoghq.script":                sets.New[string]("testConnection", "enrichScript"),
				"com.datadoghq.gitlab.users":          sets.New[string]("testConnection"),
				"com.datadoghq.kubernetes.core":       sets.New[string]("testConnection"),
				"com.datadoghq.remoteaction":          sets.New[string]("testConnection"),
				"com.datadoghq.remoteaction.internal": sets.New[string]("prepareEncryption"),
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
				mockConfig.SetInTest("site", tt.site)
			}
			if tt.ddURL != "" {
				mockConfig.SetInTest("dd_url", tt.ddURL)
			}

			// Set minimal required PAR config to avoid errors
			mockConfig.SetInTest(setup.PARPrivateKey, "")
			mockConfig.SetInTest(setup.PARUrn, "")

			// Call FromDDConfig
			cfg, err := FromDDConfig(mockConfig, nil)
			require.NoError(t, err)

			// Verify both DDHost and DatadogSite are set correctly
			assert.Equal(t, tt.expectedDDHost, cfg.DDHost, "DDHost mismatch")
			assert.Equal(t, tt.expectedDDSite, cfg.DatadogSite, "DatadogSite mismatch")

			// Verify DDApiHost is derived from site, not from dd_url
			assert.Equal(t, "api."+tt.expectedDDSite, cfg.DDApiHost, "DDApiHost should be api.<site>")
		})
	}
}

func TestFromDDConfigMetricsClient(t *testing.T) {
	providedClient := &statsd.NoOpClient{}
	tests := []struct {
		name     string
		client   statsd.ClientInterface
		wantSame statsd.ClientInterface
		wantNoOp bool
	}{
		{
			name:     "uses provided metrics client",
			client:   providedClient,
			wantSame: providedClient,
		},
		{
			name:     "defaults nil metrics client to no-op",
			wantNoOp: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := configmock.New(t)
			mockConfig.SetInTest(setup.PARPrivateKey, "")
			mockConfig.SetInTest(setup.PARUrn, "")

			cfg, err := FromDDConfig(mockConfig, tt.client)

			require.NoError(t, err)
			if tt.wantSame != nil {
				assert.Same(t, tt.wantSame, cfg.MetricsClient)
			}
			if tt.wantNoOp {
				assert.IsType(t, &statsd.NoOpClient{}, cfg.MetricsClient)
			}
		})
	}
}

func TestMakeActionsAllowlistDefaultActionsEnabled(t *testing.T) {
	t.Run("cluster agent default actions are included when default_actions_enabled is true", func(t *testing.T) {
		flavor.SetFlavor(flavor.ClusterAgent)
		defer flavor.SetFlavor(flavor.DefaultAgent)

		mockConfig := configmock.New(t)
		mockConfig.SetInTest(setup.PARActionsAllowlist, []string{})
		mockConfig.SetInTest(setup.PARDefaultActionsEnabled, true)

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
		mockConfig.SetInTest(setup.PARActionsAllowlist, []string{})
		mockConfig.SetInTest(setup.PARDefaultActionsEnabled, true)

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
		mockConfig.SetInTest(setup.PARActionsAllowlist, []string{})
		mockConfig.SetInTest(setup.PARDefaultActionsEnabled, false)

		allowlist := makeActionsAllowlist(mockConfig)

		assert.Empty(t, allowlist)
	})

	t.Run("cluster agent default actions merge with explicit allowlist", func(t *testing.T) {
		flavor.SetFlavor(flavor.ClusterAgent)
		defer flavor.SetFlavor(flavor.DefaultAgent)

		mockConfig := configmock.New(t)
		mockConfig.SetInTest(setup.PARActionsAllowlist, []string{"com.datadoghq.http.sendRequest"})
		mockConfig.SetInTest(setup.PARDefaultActionsEnabled, true)

		allowlist := makeActionsAllowlist(mockConfig)

		assert.True(t, allowlist["com.datadoghq.kubernetes.apps"].Has("listDeployment"))
		assert.True(t, allowlist["com.datadoghq.http"].Has("sendRequest"))
	})

	t.Run("explicit allowlist works without default actions", func(t *testing.T) {
		mockConfig := configmock.New(t)
		mockConfig.SetInTest(setup.PARActionsAllowlist, []string{"com.datadoghq.http.sendRequest"})
		mockConfig.SetInTest(setup.PARDefaultActionsEnabled, false)

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
	mockConfig.SetInTest(setup.PARPrivateKey, "")
	mockConfig.SetInTest(setup.PARUrn, "")

	cfg, err := FromDDConfig(mockConfig, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"/"}, cfg.RShellAllowedPaths)
}

func TestFromDDConfigPARRestrictedShellAllowedPathsSet(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetInTest(setup.PARPrivateKey, "")
	mockConfig.SetInTest(setup.PARUrn, "")
	mockConfig.SetInTest(setup.PARRestrictedShellAllowedPaths, []string{"/var/log", "/tmp"})

	cfg, err := FromDDConfig(mockConfig, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"/var/log", "/tmp"}, cfg.RShellAllowedPaths)
}

func TestFromDDConfigPARRestrictedShellAllowedPathsEmpty(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetInTest(setup.PARPrivateKey, "")
	mockConfig.SetInTest(setup.PARUrn, "")
	mockConfig.SetInTest(setup.PARRestrictedShellAllowedPaths, []string{})

	cfg, err := FromDDConfig(mockConfig, nil)
	require.NoError(t, err)
	// Explicit empty list remains distinct from the unset case above.
	assert.NotNil(t, cfg.RShellAllowedPaths)
	assert.Empty(t, cfg.RShellAllowedPaths)
}

func TestFromDDConfigPARRestrictedShellAllowedCommandsUnset(t *testing.T) {
	// Unset key: the registered default is ["rshell:*"], the wildcard
	// sentinel that admits every backend command in the rshell namespace.
	// The transform returns it verbatim.
	mockConfig := configmock.New(t)
	mockConfig.SetInTest(setup.PARPrivateKey, "")
	mockConfig.SetInTest(setup.PARUrn, "")

	cfg, err := FromDDConfig(mockConfig, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"rshell:*"}, cfg.RShellAllowedCommands)
}

func TestFromDDConfigPARRestrictedShellPrivilegedDefaultsAndOverrides(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetInTest(setup.PARPrivateKey, "")
	mockConfig.SetInTest(setup.PARUrn, "")

	cfg, err := FromDDConfig(mockConfig, nil)
	require.NoError(t, err)
	assert.False(t, cfg.RShellPrivilegedEnabled)
	assert.Equal(t, setup.RShellPrivilegedSocketDefault, cfg.RShellPrivilegedSocket)

	mockConfig.SetInTest(setup.PARRestrictedShellPrivilegedEnabled, true)
	mockConfig.SetInTest(setup.PARRestrictedShellPrivilegedSocket, "/run/custom-rshell.sock")
	cfg, err = FromDDConfig(mockConfig, nil)
	require.NoError(t, err)
	assert.True(t, cfg.RShellPrivilegedEnabled)
	assert.Equal(t, "/run/custom-rshell.sock", cfg.RShellPrivilegedSocket)
}

func TestFromDDConfigPARRestrictedShellAllowedCommandsSet(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetInTest(setup.PARPrivateKey, "")
	mockConfig.SetInTest(setup.PARUrn, "")
	mockConfig.SetInTest(setup.PARRestrictedShellAllowedCommands, []string{"cat", "ls"})

	cfg, err := FromDDConfig(mockConfig, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"cat", "ls"}, cfg.RShellAllowedCommands)
}

func TestFromDDConfigPARRestrictedShellAllowedCommandsEmpty(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetInTest(setup.PARPrivateKey, "")
	mockConfig.SetInTest(setup.PARUrn, "")
	mockConfig.SetInTest(setup.PARRestrictedShellAllowedCommands, []string{})

	cfg, err := FromDDConfig(mockConfig, nil)
	require.NoError(t, err)
	// Explicit empty list remains distinct from the unset case above.
	assert.NotNil(t, cfg.RShellAllowedCommands)
	assert.Empty(t, cfg.RShellAllowedCommands)
}

// TestFromDDConfigPARRestrictedShellAllowedPathsEmptyYAML pins the
// transform contract for `allowed_paths: []`: GetStringSlice returns a
// nil slice for the explicit YAML empty list, and the transform forwards
// that as-is.
// The slice value here is "no entries" regardless of nil/non-nil shape.
func TestFromDDConfigPARRestrictedShellAllowedPathsEmptyYAML(t *testing.T) {
	yaml := `
private_action_runner:
  restricted_shell:
    allowed_paths: []
`
	mockConfig := configmock.NewFromYAML(t, yaml)

	cfg, err := FromDDConfig(mockConfig, nil)
	require.NoError(t, err)
	assert.Empty(t, cfg.RShellAllowedPaths, "YAML [] must surface as an empty slice")
}

func TestFromDDConfigPARRestrictedShellAllowedCommandsEmptyYAML(t *testing.T) {
	yaml := `
private_action_runner:
  restricted_shell:
    allowed_commands: []
`
	mockConfig := configmock.NewFromYAML(t, yaml)

	cfg, err := FromDDConfig(mockConfig, nil)
	require.NoError(t, err)
	assert.Empty(t, cfg.RShellAllowedCommands, "YAML [] must surface as an empty slice")
}

func TestFromDDConfigPARRestrictedShellAllowedPathsPassesThroughFileEntries(t *testing.T) {
	// Config entries are parsed and returned as written; path normalization and
	// containment matching happen in the rshell bundle.
	tmpDir := t.TempDir()
	fp := filepath.Join(tmpDir, "file.txt")
	require.NoError(t, os.WriteFile(fp, []byte("x"), 0o600))

	mockConfig := configmock.New(t)
	mockConfig.SetInTest(setup.PARPrivateKey, "")
	mockConfig.SetInTest(setup.PARUrn, "")
	mockConfig.SetInTest(setup.PARRestrictedShellAllowedPaths, []string{tmpDir, fp})

	cfg, err := FromDDConfig(mockConfig, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{tmpDir, fp}, cfg.RShellAllowedPaths)
}

func TestFromDDConfigPARRestrictedShellAllowedPathsPassesThroughBackslash(t *testing.T) {
	// Backslash-containing entries are preserved in the returned slice.
	mockConfig := configmock.New(t)
	mockConfig.SetInTest(setup.PARPrivateKey, "")
	mockConfig.SetInTest(setup.PARUrn, "")
	mockConfig.SetInTest(setup.PARRestrictedShellAllowedPaths, []string{`C:\Data`, "/var/log"})

	cfg, err := FromDDConfig(mockConfig, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{`C:\Data`, "/var/log"}, cfg.RShellAllowedPaths)
}

func TestFromDDConfigPARRestrictedShellAllowedPathsWarnsForBackslash(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetInTest(setup.PARPrivateKey, "")
	mockConfig.SetInTest(setup.PARUrn, "")
	mockConfig.SetInTest(setup.PARRestrictedShellAllowedPaths, []string{`C:\Data`, "/var/log"})

	logs := captureTransformWarnings(t, func() {
		_, err := FromDDConfig(mockConfig, nil)
		require.NoError(t, err)
	})

	assert.Contains(t, logs, setup.PARRestrictedShellAllowedPaths)
	assert.Contains(t, logs, `C:\\Data`)
	assert.Contains(t, logs, "contains a backslash")
	assert.Contains(t, logs, "only forward-slash paths are supported")
	assert.NotContains(t, logs, "/var/log")
}

func TestFromDDConfigPARRestrictedShellAllowedPathsWarnsForNonDirectory(t *testing.T) {
	tmpDir := filepath.ToSlash(t.TempDir())
	fp := filepath.ToSlash(filepath.Join(tmpDir, "file.txt"))
	require.NoError(t, os.WriteFile(fp, []byte("x"), 0o600))

	mockConfig := configmock.New(t)
	mockConfig.SetInTest(setup.PARPrivateKey, "")
	mockConfig.SetInTest(setup.PARUrn, "")
	mockConfig.SetInTest(setup.PARRestrictedShellAllowedPaths, []string{tmpDir, fp})

	logs := captureTransformWarnings(t, func() {
		_, err := FromDDConfig(mockConfig, nil)
		require.NoError(t, err)
	})

	assert.Contains(t, logs, setup.PARRestrictedShellAllowedPaths)
	assert.Contains(t, logs, fp)
	assert.Contains(t, logs, "is not a directory")
	assert.Contains(t, logs, "Use the containing directory instead")
	assert.NotContains(t, logs, `entry "`+tmpDir+`" is not a directory`)
}

func TestFromDDConfigPARRestrictedShellAllowedCommandsPassesThroughUnnamespaced(t *testing.T) {
	// Unnamespaced entries are preserved in the returned slice.
	mockConfig := configmock.New(t)
	mockConfig.SetInTest(setup.PARPrivateKey, "")
	mockConfig.SetInTest(setup.PARUrn, "")
	mockConfig.SetInTest(setup.PARRestrictedShellAllowedCommands, []string{"cat", "rshell:ls"})

	cfg, err := FromDDConfig(mockConfig, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"cat", "rshell:ls"}, cfg.RShellAllowedCommands)
}

func TestFromDDConfigPARRestrictedShellAllowedCommandsWarnsForUnnamespaced(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetInTest(setup.PARPrivateKey, "")
	mockConfig.SetInTest(setup.PARUrn, "")
	mockConfig.SetInTest(setup.PARRestrictedShellAllowedCommands, []string{"cat", "rshell:ls"})

	logs := captureTransformWarnings(t, func() {
		_, err := FromDDConfig(mockConfig, nil)
		require.NoError(t, err)
	})

	assert.Contains(t, logs, setup.PARRestrictedShellAllowedCommands)
	assert.Contains(t, logs, `"cat"`)
	assert.Contains(t, logs, `"rshell:"`)
	assert.Contains(t, logs, `"rshell:cat"`)
	assert.NotContains(t, logs, `"rshell:ls"`)
}

func TestFromDDConfigPARRestrictedShellAllowedCommandsDefaultDoesNotWarn(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetInTest(setup.PARPrivateKey, "")
	mockConfig.SetInTest(setup.PARUrn, "")

	logs := captureTransformWarnings(t, func() {
		_, err := FromDDConfig(mockConfig, nil)
		require.NoError(t, err)
	})

	assert.Empty(t, logs)
}

func TestFromDDConfigPARRestrictedShellAllowedAbsentYAML(t *testing.T) {
	// No restricted_shell block at all: both axes fall back to their registered
	// defaults.
	yaml := `
private_action_runner:
  enabled: true
`
	mockConfig := configmock.NewFromYAML(t, yaml)

	cfg, err := FromDDConfig(mockConfig, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"/"}, cfg.RShellAllowedPaths)
	assert.Equal(t, []string{"rshell:*"}, cfg.RShellAllowedCommands)
}

func TestNewMetricsClient(t *testing.T) {
	createErr := errors.New("permission denied")
	tests := []struct {
		name           string
		port           int
		bindHost       string
		createErr      error
		wantErr        string
		wantNoOp       bool
		wantHost       string
		wantPort       int
		wantSameClient bool
	}{
		{
			name:           "uses configured bind host and dogstatsd port",
			port:           8126,
			bindHost:       "127.0.0.1",
			wantHost:       "127.0.0.1",
			wantPort:       8126,
			wantSameClient: true,
		},
		{
			name:      "returns no-op and error when DogStatsD client creation fails",
			port:      8126,
			bindHost:  "127.0.0.1",
			createErr: createErr,
			wantErr:   "failed to create DogStatsD client",
			wantNoOp:  true,
			wantHost:  "127.0.0.1",
			wantPort:  8126,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("STATSD_URL", "")
			mockConfig := configmock.New(t)
			mockConfig.SetInTest("dogstatsd_port", tt.port)
			if tt.bindHost != "" {
				mockConfig.SetInTest("bind_host", tt.bindHost)
			}

			wantClient := &statsd.NoOpClient{}
			statsdComp := &recordingStatsdComponent{client: wantClient, err: tt.createErr}
			got, err := NewMetricsClient(mockConfig, statsdComp)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tt.wantErr)
				assert.ErrorIs(t, err, tt.createErr)
			} else {
				require.NoError(t, err)
			}
			if tt.wantSameClient {
				assert.Same(t, wantClient, got)
			}
			if tt.wantNoOp {
				assert.IsType(t, &statsd.NoOpClient{}, got)
			}
			assert.Equal(t, "host_port", statsdComp.call)
			assert.Equal(t, tt.wantHost, statsdComp.host)
			assert.Equal(t, tt.wantPort, statsdComp.port)
			assert.Empty(t, statsdComp.addr)
		})
	}
}

type recordingStatsdComponent struct {
	client statsd.ClientInterface
	err    error
	call   string
	addr   string
	host   string
	port   int
}

var _ statsdcomp.Component = (*recordingStatsdComponent)(nil)

func (r *recordingStatsdComponent) Get() (statsd.ClientInterface, error) {
	r.call = "get"
	return r.client, r.err
}

func (r *recordingStatsdComponent) Create(_ ...statsd.Option) (statsd.ClientInterface, error) {
	r.call = "create"
	return r.client, r.err
}

func (r *recordingStatsdComponent) CreateForAddr(addr string, _ ...statsd.Option) (statsd.ClientInterface, error) {
	r.call = "addr"
	r.addr = addr
	return r.client, r.err
}

func (r *recordingStatsdComponent) CreateForHostPort(host string, port int, _ ...statsd.Option) (statsd.ClientInterface, error) {
	r.call = "host_port"
	r.host = host
	r.port = port
	return r.client, r.err
}

func captureTransformWarnings(t *testing.T, fn func()) string {
	t.Helper()

	var logBuffer bytes.Buffer
	logWriter := bufio.NewWriter(&logBuffer)
	logger, err := log.LoggerFromWriterWithMinLevelAndLvlMsgFormat(logWriter, log.WarnLvl)
	require.NoError(t, err)

	previousLogger := log.Default()
	t.Cleanup(func() {
		log.SetupLogger(previousLogger, "debug")
	})
	log.SetupLogger(logger, "warn")

	fn()

	require.NoError(t, logWriter.Flush())
	return logBuffer.String()
}
