// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package commands

import (
	"context"
	"os"
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

func TestSetupCommandHasHumanReadableAnnotation(t *testing.T) {
	cmd := setupCommand()
	assert.Equal(t, "true", cmd.Annotations[AnnotationHumanReadableErrors],
		"setup command should have human-readable-errors annotation")
}
