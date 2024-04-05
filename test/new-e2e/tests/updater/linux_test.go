// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package updater contains tests for the updater package
package updater

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"gopkg.in/yaml.v2"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	confDir     = "/etc/datadog-agent"
	logDir      = "/var/log/datadog"
	locksDir    = "/var/run/datadog-packages"
	packagesDir = "/opt/datadog-packages"
	installDir  = "/opt/datadog/updater"
)

type vmUpdaterSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestUpdaterSuite(t *testing.T) {
	e2e.Run(t, &vmUpdaterSuite{}, e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake(
		awshost.WithUpdater(),
		awshost.WithEC2InstanceOptions(ec2.WithOSArch(os.UbuntuDefault, os.ARM64Arch)),
	)))
}

func (v *vmUpdaterSuite) TestUserGroupsCreation() {
	// users exist and is a system user
	require.Equal(v.T(), "/usr/sbin/nologin\n", v.Env().RemoteHost.MustExecute(`getent passwd dd-agent | cut -d: -f7`), "unexpected: user does not exist or is not a system user")
	require.Equal(v.T(), "/usr/sbin/nologin\n", v.Env().RemoteHost.MustExecute(`getent passwd dd-updater | cut -d: -f7`), "unexpected: user does not exist or is not a system user")
	require.Equal(v.T(), "dd-updater\n", v.Env().RemoteHost.MustExecute(`getent group dd-updater | cut -d":" -f1`), "unexpected: group does not exist")
	require.Equal(v.T(), "dd-agent\n", v.Env().RemoteHost.MustExecute(`getent group dd-agent | cut -d":" -f1`), "unexpected: group does not exist")
	require.Equal(v.T(), "dd-updater dd-agent\n", v.Env().RemoteHost.MustExecute("id -Gn dd-updater"), "dd-updater not in correct groups")
}

func (v *vmUpdaterSuite) TestSharedAgentDirs() {
	for _, dir := range []string{confDir, logDir} {
		require.Equal(v.T(), "dd-agent\n", v.Env().RemoteHost.MustExecute(`stat -c "%U" `+dir))
		require.Equal(v.T(), "dd-agent\n", v.Env().RemoteHost.MustExecute(`stat -c "%G" `+dir))
		require.Equal(v.T(), "drwxrwxr-x\n", v.Env().RemoteHost.MustExecute(`stat -c "%A" `+dir))
	}
}

func (v *vmUpdaterSuite) TestUpdaterDirs() {
	for _, dir := range []string{locksDir, packagesDir, installDir} {
		require.Equal(v.T(), "dd-updater\n", v.Env().RemoteHost.MustExecute(`stat -c "%U" `+dir))
		require.Equal(v.T(), "dd-updater\n", v.Env().RemoteHost.MustExecute(`stat -c "%G" `+dir))
	}
	require.Equal(v.T(), "drwxrwxrwx\n", v.Env().RemoteHost.MustExecute(`stat -c "%A" `+locksDir))
	require.Equal(v.T(), "drwxr-xr-x\n", v.Env().RemoteHost.MustExecute(`stat -c "%A" `+packagesDir))
	require.Equal(v.T(), "drwxr-xr-x\n", v.Env().RemoteHost.MustExecute(`stat -c "%A" `+installDir))
}

func (v *vmUpdaterSuite) TestAgentUnitsLoaded() {
	stableUnits := []string{
		"datadog-agent.service",
		"datadog-agent-trace.service",
		"datadog-agent-process.service",
		"datadog-agent-sysprobe.service",
		"datadog-agent-security.service",
	}
	for _, unit := range stableUnits {
		require.Equal(v.T(), "enabled\n", v.Env().RemoteHost.MustExecute(fmt.Sprintf(`systemctl is-enabled %s`, unit)))
	}
}

func (v *vmUpdaterSuite) TestExperimentCrash() {
	host := v.Env().RemoteHost
	t := v.T()
	startTime := getMonotonicTimestamp(t, host)
	v.Env().RemoteHost.MustExecute(`sudo systemctl start datadog-agent-exp --no-block`)
	res := getJournalDOnCondition(t, host, startTime, stopCondition([]JournaldLog{
		{Unit: "datadog-agent.service", Message: "Started"},
	}))
	require.True(t, verifyLogs(res, []JournaldLog{
		{Unit: "datadog-agent.service", Message: "Stopping"},
		{Unit: "datadog-agent.service", Message: "Stopped"},
		{Unit: "datadog-agent-exp.service", Message: "Starting"},
		{Unit: "datadog-agent-exp.service", Message: "Failed"},
		{Unit: "datadog-agent.service", Message: "Started"},
	}), fmt.Sprintf("unexpected logs: %v", res))
}

func (v *vmUpdaterSuite) TestPurgeAndInstallAgent() {
	host := v.Env().RemoteHost
	host.MustExecute("sudo /opt/datadog/updater/bin/updater/updater purge")
	stableUnits := []string{
		"datadog-agent.service",
		"datadog-agent-trace.service",
		"datadog-agent-process.service",
		"datadog-agent-sysprobe.service",
		"datadog-agent-security.service",
	}
	for _, unit := range stableUnits {
		_, err := host.Execute(fmt.Sprintf(`systemctl is-enabled %s`, unit))
		require.Equal(
			v.T(),
			fmt.Sprintf("Failed to get unit file state for %s: No such file or directory\n: Process exited with status 1", unit),
			err.Error(),
		)
	}

	// dir exists
	host.MustExecute(`test -d /opt/datadog-packages`)
	// dir does not exist
	_, err := host.Execute(`test -d /opt/datadog-packages/datadog-agent`)
	require.NotNil(v.T(), err)

	// agent symlink does not exist
	_, err = host.Execute(`test -L /usr/bin/datadog-agent`)
	require.NotNil(v.T(), err)

	// install info files do not exist
	for _, file := range []string{"install_info", "install.json"} {
		exists, _ := host.FileExists(filepath.Join(confDir, file))
		assert.False(v.T(), exists)
	}

	// bootstrap
	host.MustExecute("sudo /opt/datadog/updater/bin/updater/updater bootstrap -P datadog-agent")

	// assert agent symlink
	_ = host.MustExecute(`test -L /usr/bin/datadog-agent`)
	require.Equal(v.T(), "/usr/bin/datadog-agent\n", host.MustExecute("which datadog-agent"))
	binPath := host.MustExecute("readlink -f $(which datadog-agent)")
	assert.True(v.T(), strings.HasPrefix(binPath, "/opt/datadog-packages/datadog-agent/7."))
	assert.True(v.T(), strings.HasSuffix(binPath, "/bin/agent/agent\n"))

	// assert install info files
	for _, file := range []string{"install_info", "install.json"} {
		exists, _ := host.FileExists(filepath.Join(confDir, file))
		assert.True(v.T(), exists)
	}
	assertInstallMethod(v.T(), host)

	// assert file ownerships
	agentDir := "/opt/datadog-packages/datadog-agent"
	require.Equal(v.T(), "dd-updater\n", host.MustExecute(`stat -c "%U" `+agentDir))
	require.Equal(v.T(), "dd-updater\n", host.MustExecute(`stat -c "%G" `+agentDir))
	require.Equal(v.T(), "drwxr-xr-x\n", host.MustExecute(`stat -c "%A" `+agentDir))
	require.Equal(v.T(), "1\n", host.MustExecute(`sudo ls -l /opt/datadog-packages/datadog-agent | awk '$9 != "stable" && $3 == "dd-agent" && $4 == "dd-agent"' | wc -l`))

	// assert units
	for _, unit := range stableUnits {
		require.Equal(v.T(), "enabled\n", v.Env().RemoteHost.MustExecute(fmt.Sprintf(`systemctl is-enabled %s`, unit)))
	}
}

func assertInstallMethod(t *testing.T, host *components.RemoteHost) {
	rawYaml, err := host.ReadFile(filepath.Join(confDir, "install_info"))
	assert.Nil(t, err)
	var config Config
	require.Nil(t, yaml.Unmarshal(rawYaml, &config))

	assert.Equal(t, "updater_package", config.InstallMethod["installer_version"])
	assert.Equal(t, "dpkg", config.InstallMethod["tool"])
	assert.True(t, "" != config.InstallMethod["tool_version"])
}

// Config yaml struct
type Config struct {
	InstallMethod map[string]string `yaml:"install_method"`
}
