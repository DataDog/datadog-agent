// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/exec"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths/testutil"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
)

const (
	testCmdEnv = "DD_TEST_CMD"
)

func TestMain(m *testing.M) {
	if _, isSet := os.LookupEnv(testCmdEnv); isSet {
		MockInstaller = NewInstallerMock()
		cmd := &cobra.Command{
			Use: "installer [command]",
		}
		cmd.AddGroup(
			&cobra.Group{
				ID:    "installer",
				Title: "Installer Commands",
			},
			&cobra.Group{
				ID:    "apm",
				Title: "APM Commands",
			},
			&cobra.Group{
				ID:    "extension",
				Title: "Extensions Commands",
			},
		)
		cmd.AddCommand(RootCommands()...)
		cmd.AddCommand(UnprivilegedCommands()...)
		err := cmd.Execute()
		if err != nil {
			panic(err)
		}
		return
	}
	os.Setenv(testCmdEnv, "true")
	cleanup := setupTestPaths()
	defer cleanup()
	os.Exit(m.Run())
}

func TestState(t *testing.T) {
	installerBinary, err := exec.GetExecutable()
	assert.NoError(t, err)
	env := &env.Env{
		IsFromDaemon: true,
	}
	installer := exec.NewInstallerExec(env, installerBinary)

	res, err := installer.State(context.TODO(), "datadog-agent")
	assert.NoError(t, err)

	expected := repository.State{
		Stable:     "7.31.0",
		Experiment: "7.32.0",
	}

	assert.Equal(t, expected, res)
}

func TestConfigState(t *testing.T) {
	installerBinary, err := exec.GetExecutable()
	assert.NoError(t, err)
	env := &env.Env{
		IsFromDaemon: true,
	}
	installer := exec.NewInstallerExec(env, installerBinary)

	res, err := installer.ConfigState(context.TODO(), "datadog-agent")
	assert.NoError(t, err)

	expected := repository.State{
		Stable:     "abc-def-hij",
		Experiment: "",
	}

	assert.Equal(t, expected, res)
}

func TestConfigAndPackageStates(t *testing.T) {
	installerBinary, err := exec.GetExecutable()
	assert.NoError(t, err)
	env := &env.Env{
		IsFromDaemon: true,
	}
	installer := exec.NewInstallerExec(env, installerBinary)

	res, err := installer.ConfigAndPackageStates(context.TODO())
	assert.NoError(t, err)

	expected := &repository.PackageStates{
		ConfigStates: map[string]repository.State{
			"datadog-agent": {
				Stable:     "abc-def-hij",
				Experiment: "",
			},
		},
		States: map[string]repository.State{
			"datadog-agent": {
				Stable:     "7.31.0",
				Experiment: "7.32.0",
			},
		},
	}

	assert.Equal(t, expected, res)
}

// testEnvAccessor allows tests to read any field of env.Env by name.
type testEnvAccessor struct{ e *env.Env }

func (a testEnvAccessor) GetEnv(field string) string {
	v := reflect.ValueOf(a.e).Elem()
	f := v.FieldByName(field)
	if !f.IsValid() {
		return fmt.Sprintf("<unknown field %q>", field)
	}
	return fmt.Sprintf("%v", f.Interface())
}

type testCmd struct {
	*cmd
	Test testEnvAccessor
}

// newTestingCmd creates a cmd via newCmd and exposes its env through Test.GetEnv.
func newTestingCmd(t *testing.T) *testCmd {
	t.Helper()
	c, err := newCmd("unit_test", withQuiet())
	if err != nil {
		t.Fatal(err)
	}
	return &testCmd{cmd: c, Test: testEnvAccessor{e: c.env}}
}

func TestNewCmd(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string            // written to temp dir as datadog.yaml; empty means no file
		envVars map[string]string // set via t.Setenv before creating the cmd
		checks  map[string]string // env.Env field name -> expected value
	}{
		{
			name: "no config no env vars",
			checks: map[string]string{
				"RegistryAuthOverride": "",
				"RegistryOverride":     "",
				"RegistryUsername":     "",
				"RegistryPassword":     "",
			},
		},
		{
			name: "env vars only",
			envVars: map[string]string{
				"DD_INSTALLER_REGISTRY_AUTH":     "gcr",
				"DD_INSTALLER_REGISTRY_URL":      "env-registry.example.com",
				"DD_INSTALLER_REGISTRY_USERNAME": "env-user",
				"DD_INSTALLER_REGISTRY_PASSWORD": "env-pass",
			},
			checks: map[string]string{
				"RegistryAuthOverride": "gcr",
				"RegistryOverride":     "env-registry.example.com",
				"RegistryUsername":     "env-user",
				"RegistryPassword":     "env-pass",
			},
		},
		{
			name: "datadog.yaml only",
			yaml: `
installer:
  registry:
    url:      yaml-registry.example.com
    auth:     gcr
    username: yaml-user
    password: yaml-pass
`,
			checks: map[string]string{
				"RegistryAuthOverride": "gcr",
				"RegistryOverride":     "yaml-registry.example.com",
				"RegistryUsername":     "yaml-user",
				"RegistryPassword":     "yaml-pass",
			},
		},
		{
			name: "env vars take precedence over datadog.yaml",
			yaml: `
installer:
  registry:
    url:      yaml-registry.example.com
    auth:     yaml-auth
    username: yaml-user
    password: yaml-pass
`,
			envVars: map[string]string{
				"DD_INSTALLER_REGISTRY_AUTH": "env-auth",
				"DD_INSTALLER_REGISTRY_URL":  "env-registry.example.com",
			},
			checks: map[string]string{
				"RegistryAuthOverride": "env-auth",
				"RegistryOverride":     "env-registry.example.com",
				// remaining fields not set by env var are filled from YAML
				"RegistryUsername": "yaml-user",
				"RegistryPassword": "yaml-pass",
			},
		},
		{
			name: "partial yaml fills only provided fields",
			yaml: `
installer:
  registry:
    auth: gcr
`,
			checks: map[string]string{
				"RegistryAuthOverride": "gcr",
				"RegistryOverride":     "",
				"RegistryUsername":     "",
				"RegistryPassword":     "",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := testutil.TrustedDir(t)
			agentConfigDir = dir
			t.Cleanup(func() { agentConfigDir = paths.AgentConfigDir })

			if tc.yaml != "" {
				err := os.WriteFile(filepath.Join(dir, "datadog.yaml"), []byte(tc.yaml), 0644)
				assert.NoError(t, err)
			}
			for k, v := range tc.envVars {
				t.Setenv(k, v)
			}

			cmd := newTestingCmd(t)
			defer cmd.stop(nil)

			for field, want := range tc.checks {
				assert.Equal(t, want, cmd.Test.GetEnv(field), "field %s", field)
			}
		})
	}
}

func TestTelemetryConfigUsesAgentConfigDir(t *testing.T) {
	originalConfigDir := agentConfigDir
	t.Cleanup(func() { agentConfigDir = originalConfigDir })
	agentConfigDir = testutil.TrustedDir(t)
	if err := os.WriteFile(filepath.Join(agentConfigDir, "datadog.yaml"),
		[]byte("api_key: synthetic-key\nsite: custom.example\n"), 0600); err != nil {
		t.Fatal(err)
	}
	config, err := telemetryConfig()
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, telemetryConfigFields{APIKey: "synthetic-key", Site: "custom.example"}, config)
}

func TestSetupCommandHasHumanReadableAnnotation(t *testing.T) {
	cmd := setupCommand()
	assert.Equal(t, "true", cmd.Annotations[AnnotationHumanReadableErrors],
		"setup command should have human-readable-errors annotation")
}

func unsetEnvForTest(t *testing.T, key string) {
	t.Setenv(key, "")
	require.NoError(t, os.Unsetenv(key))
}

func TestTelemetryEndpoint(t *testing.T) {
	enabled, disabled := true, false
	tests := []struct {
		name            string
		yamlURL         string
		yamlEnabled     *bool
		envURL          *string
		envEnabled      *string
		expectedURL     string
		expectedEnabled bool
	}{
		{name: "defaults", expectedEnabled: true},
		{name: "yaml url", yamlURL: "https://relay.example:3840", expectedURL: "https://relay.example:3840", expectedEnabled: true},
		{name: "yaml disabled", yamlEnabled: &disabled, expectedEnabled: false},
		{name: "yaml enabled", yamlEnabled: &enabled, expectedEnabled: true},
		{name: "env url overrides yaml", yamlURL: "https://yaml.example", envURL: telemetryStrPtr("https://env.example"), expectedURL: "https://env.example", expectedEnabled: true},
		{name: "empty env url keeps yaml", yamlURL: "https://yaml.example", envURL: telemetryStrPtr(""), expectedURL: "https://yaml.example", expectedEnabled: true},
		{name: "env disabled overrides yaml", yamlEnabled: &enabled, envEnabled: telemetryStrPtr("false"), expectedEnabled: false},
		{name: "invalid env enabled keeps yaml", yamlEnabled: &disabled, envEnabled: telemetryStrPtr("nope"), expectedEnabled: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			unsetEnvForTest(t, "DD_APM_TELEMETRY_DD_URL")
			unsetEnvForTest(t, "DD_APM_TELEMETRY_ENABLED")
			if tt.envURL != nil {
				t.Setenv("DD_APM_TELEMETRY_DD_URL", *tt.envURL)
			}
			if tt.envEnabled != nil {
				t.Setenv("DD_APM_TELEMETRY_ENABLED", *tt.envEnabled)
			}
			var config telemetryConfigFields
			config.APMConfig.Telemetry.DDURL = tt.yamlURL
			config.APMConfig.Telemetry.Enabled = tt.yamlEnabled

			url, enabled := config.telemetryEndpoint()

			assert.Equal(t, tt.expectedURL, url)
			assert.Equal(t, tt.expectedEnabled, enabled)
		})
	}
}

func TestTelemetryConfigReadsAPMTelemetry(t *testing.T) {
	originalConfigDir := agentConfigDir
	t.Cleanup(func() { agentConfigDir = originalConfigDir })
	agentConfigDir = testutil.TrustedDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(agentConfigDir, "datadog.yaml"),
		[]byte("api_key: synthetic-key\napm_config:\n  telemetry:\n    dd_url: https://relay.example:3840\n    enabled: false\n"), 0600))

	config, err := telemetryConfig()
	require.NoError(t, err)

	assert.Equal(t, "https://relay.example:3840", config.APMConfig.Telemetry.DDURL)
	require.NotNil(t, config.APMConfig.Telemetry.Enabled)
	assert.False(t, *config.APMConfig.Telemetry.Enabled)
}

func telemetryStrPtr(s string) *string {
	return &s
}
