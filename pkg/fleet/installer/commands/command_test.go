// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package commands

import (
	"context"
	"fmt"
	"os"
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
func newTestingCmd() *testCmd {
	c := newCmd("unit_test", withQuiet())
	return &testCmd{cmd: c, Test: testEnvAccessor{e: c.env}}
}

// TestNewCmd asserts that env-var settings flow through newCmd into the
// resolved *env.Env. The installer is env-var-only; any yaml-sourced
// configuration is the caller's responsibility to translate into DD_* env
// vars before invoking the installer.
func TestNewCmd(t *testing.T) {
	tests := []struct {
		name    string
		envVars map[string]string
		checks  map[string]string
	}{
		{
			name: "no env vars",
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
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.envVars {
				t.Setenv(k, v)
			}

			cmd := newTestingCmd()
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
