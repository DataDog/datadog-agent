// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package commands

import (
	"context"
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/exec"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

const (
	testCmdEnv = "DD_TEST_CMD"
)

func TestMain(m *testing.M) {
	if _, isSet := os.LookupEnv(testCmdEnv); isSet {
		mockInstaller = newInstallerMock()
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
	os.Exit(m.Run())
}

type installerMock struct{}

func newInstallerMock() installer.Installer {
	return &installerMock{}
}

func (m *installerMock) IsInstalled(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func (m *installerMock) AvailableDiskSpace() (uint64, error) {
	return 0, nil
}

func (m *installerMock) State(_ context.Context, pkg string) (repository.State, error) {
	if pkg == "datadog-agent" {
		return repository.State{
			Stable:     "7.31.0",
			Experiment: "7.32.0",
		}, nil
	}
	return repository.State{}, nil
}

func (m *installerMock) States(_ context.Context) (map[string]repository.State, error) {
	return map[string]repository.State{
		"datadog-agent": {
			Stable:     "7.31.0",
			Experiment: "7.32.0",
		},
	}, nil
}

func (m *installerMock) ConfigState(_ context.Context, pkg string) (repository.State, error) {
	if pkg == "datadog-agent" {
		return repository.State{
			Stable:     "abc-def-hij",
			Experiment: "",
		}, nil
	}
	return repository.State{}, nil
}

func (m *installerMock) ConfigStates(_ context.Context) (map[string]repository.State, error) {
	return map[string]repository.State{
		"datadog-agent": {
			Stable:     "abc-def-hij",
			Experiment: "",
		},
	}, nil
}

func (m *installerMock) Install(_ context.Context, _ string, _ []string) error {
	return nil
}

func (m *installerMock) ForceInstall(_ context.Context, _ string, _ []string) error {
	return nil
}

func (m *installerMock) Remove(_ context.Context, _ string) error {
	return nil
}

func (m *installerMock) Purge(_ context.Context) {}

func (m *installerMock) InstallExperiment(_ context.Context, _ string) error {
	return nil
}

func (m *installerMock) RemoveExperiment(_ context.Context, _ string) error {
	return nil
}

func (m *installerMock) PromoteExperiment(_ context.Context, _ string) error {
	return nil
}

func (m *installerMock) InstallConfigExperiment(_ context.Context, _ string, _ string, _ []byte) error {
	return nil
}

func (m *installerMock) RemoveConfigExperiment(_ context.Context, _ string) error {
	return nil
}

func (m *installerMock) PromoteConfigExperiment(_ context.Context, _ string) error {
	return nil
}

func (m *installerMock) GarbageCollect(_ context.Context) error {
	return nil
}

func (m *installerMock) InstrumentAPMInjector(_ context.Context, _ string) error {
	return nil
}

func (m *installerMock) UninstrumentAPMInjector(_ context.Context, _ string) error {
	return nil
}

func (m *installerMock) Close() error {
	return nil
}

func TestStates(t *testing.T) {
	installerBinary, err := os.Executable()
	assert.NoError(t, err)
	env := &env.Env{
		IsFromDaemon: true,
	}
	installer := exec.NewInstallerExec(env, installerBinary)

	res, err := installer.States(context.TODO())
	assert.NoError(t, err)

	expected := map[string]repository.State{
		"datadog-agent": {
			Stable:     "7.31.0",
			Experiment: "7.32.0",
		},
	}

	assert.Equal(t, expected, res)
}

func TestState(t *testing.T) {
	installerBinary, err := os.Executable()
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

func TestConfigStates(t *testing.T) {
	installerBinary, err := os.Executable()
	assert.NoError(t, err)
	env := &env.Env{
		IsFromDaemon: true,
	}
	installer := exec.NewInstallerExec(env, installerBinary)

	res, err := installer.ConfigStates(context.TODO())
	assert.NoError(t, err)

	expected := map[string]repository.State{
		"datadog-agent": {
			Stable:     "abc-def-hij",
			Experiment: "",
		},
	}

	assert.Equal(t, expected, res)
}

func TestConfigState(t *testing.T) {
	installerBinary, err := os.Executable()
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
