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

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/exec"
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
func newTestingCmd(configDir string) *testCmd {
	c := newCmd("unit_test", withQuiet(), withConfigDir(configDir))
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
			dir := t.TempDir()

			if tc.yaml != "" {
				err := os.WriteFile(filepath.Join(dir, "datadog.yaml"), []byte(tc.yaml), 0644)
				assert.NoError(t, err)
			}
			for k, v := range tc.envVars {
				t.Setenv(k, v)
			}

			cmd := newTestingCmd(dir)
			defer cmd.stop(nil)

			for field, want := range tc.checks {
				assert.Equal(t, want, cmd.Test.GetEnv(field), "field %s", field)
			}
		})
	}
}

func TestSetupCommandHasHumanReadableAnnotation(t *testing.T) {
	cmd := setupCommand()
	assert.Equal(t, "true", cmd.Annotations[AnnotationHumanReadableErrors],
		"setup command should have human-readable-errors annotation")
}
