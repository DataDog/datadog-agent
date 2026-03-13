// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package check

import (
	"bytes"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/autodiscoveryimpl"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/scheduler"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	workloadfilterfxmock "github.com/DataDog/datadog-agent/comp/core/workloadfilter/fx-mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestCommand(t *testing.T) {
	commands := []*cobra.Command{
		MakeCommand(func() GlobalParams {
			// the config needs an existing config file when initializing
			config := path.Join(t.TempDir(), "datadog.yaml")
			err := os.WriteFile(config, []byte("hostname: test"), 0644)
			require.NoError(t, err)

			return GlobalParams{
				ConfFilePath: config,
			}
		}),
	}

	fxutil.TestOneShotSubcommand(t,
		commands,
		// this command has a lot of options, so just test a few
		[]string{"check", "cleopatra", "--delay", "1", "--flare"},
		run,
		func(cliParams *cliParams, _ core.BundleParams) {
			require.Equal(t, []string{"cleopatra"}, cliParams.args)
			require.Equal(t, 1, cliParams.checkDelay)
			require.True(t, cliParams.saveFlare)
		})
}

func TestGetAllCheckConfigs_CustomConfig(t *testing.T) {
	adsched := scheduler.NewController()
	ac := fxutil.Test[autodiscovery.Mock](t,
		fx.Supply(autodiscoveryimpl.MockParams{Scheduler: adsched}),
		fx.Provide(func() secrets.Component { return secretsmock.New(t) }),
		autodiscoveryimpl.MockModule(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
		core.MockBundle(),
		taggerfxmock.MockModule(),
		workloadfilterfxmock.MockModule(),
	)

	// create config file
	tempDir := t.TempDir()
	tempConfigFile, err := os.CreateTemp(tempDir, "config-*.yaml")
	require.NoError(t, err)

	// write in config file
	configYaml := `
init_config:
  abc: 123
instances:
  - def: 456
  - ghi: 789
`

	_, err = tempConfigFile.WriteString(configYaml)
	require.NoError(t, err)
	tempConfigFile.Close()

	cliParams := cliParams{
		checkName:   "custom",
		checkConfig: tempConfigFile.Name(),
	}

	checkConfigs, err := getAllCheckConfigs(ac, cliParams)
	require.NoError(t, err)
	require.Len(t, checkConfigs, 1)

	checkConfig := checkConfigs[0]
	assert.Equal(t, integration.Data("abc: 123\n"), checkConfig.InitConfig)

	require.Len(t, checkConfig.Instances, 2)
	assert.Equal(t, integration.Data("def: 456\n"), checkConfig.Instances[0])
	assert.Equal(t, integration.Data("ghi: 789\n"), checkConfig.Instances[1])
}

func TestCommandWithInstanceID(t *testing.T) {
	commands := []*cobra.Command{
		MakeCommand(func() GlobalParams {
			config := path.Join(t.TempDir(), "datadog.yaml")
			err := os.WriteFile(config, []byte("hostname: test"), 0644)
			require.NoError(t, err)

			return GlobalParams{
				ConfFilePath: config,
			}
		}),
	}

	fxutil.TestOneShotSubcommand(t,
		commands,
		[]string{"check", "http_check", "--instance-id", "3e96f922a85e2ab0"},
		run,
		func(cliParams *cliParams, _ core.BundleParams) {
			require.Equal(t, []string{"http_check"}, cliParams.args)
			require.Equal(t, "3e96f922a85e2ab0", cliParams.instanceID)
		})
}

func TestWriteCheckToFileInDir_DirectoryPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping permission test on Windows")
	}

	tempDir := t.TempDir()
	checkDir := filepath.Join(tempDir, "checks")

	// Write a check file to trigger directory creation
	checkOutput := bytes.NewBufferString("test check output")
	writeCheckToFileInDir("testcheck", checkOutput, checkDir)

	// Verify the directory was created with correct permissions
	info, err := os.Stat(checkDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	// Verify directory permissions are 0750 (rwxr-x---)
	expectedPerms := os.FileMode(0750)
	actualPerms := info.Mode().Perm()
	assert.Equal(t, expectedPerms, actualPerms, "expected directory permissions %o, got %o", expectedPerms, actualPerms)
}

func TestWriteCheckToFileInDir_FileCreated(t *testing.T) {
	tempDir := t.TempDir()
	checkDir := filepath.Join(tempDir, "checks")

	checkOutput := bytes.NewBufferString("test check output content")
	writeCheckToFileInDir("mycheck", checkOutput, checkDir)

	// Verify a file was created in the directory
	entries, err := os.ReadDir(checkDir)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	// Verify filename format: check_<name>_<timestamp>.log
	assert.Contains(t, entries[0].Name(), "check_mycheck_")
	assert.Contains(t, entries[0].Name(), ".log")
}
