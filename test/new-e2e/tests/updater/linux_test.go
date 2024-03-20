// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package updater

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"

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
		require.Equal(v.T(), "dd-agent\n", v.Env().RemoteHost.MustExecute(fmt.Sprintf(`stat -c "%U" %s`, dir)))
		require.Equal(v.T(), "dd-agent\n", v.Env().RemoteHost.MustExecute(fmt.Sprintf(`stat -c "%G" %s`, dir)))
		require.Equal(v.T(), "drwxrwxr-x\n", v.Env().RemoteHost.MustExecute(fmt.Sprintf(`stat -c "%A" %s`, dir)))
	}
}

func (v *vmUpdaterSuite) TestUpdaterDirs() {
	for _, dir := range []string{locksDir, packagesDir, installDir} {
		require.Equal(v.T(), "dd-updater\n", v.Env().RemoteHost.MustExecute(fmt.Sprintf(`stat -c "%U" %s`, dir)))
		require.Equal(v.T(), "dd-updater\n", v.Env().RemoteHost.MustExecute(fmt.Sprintf(`stat -c "%G" %s`, dir)))
	}
	require.Equal(v.T(), "drw-rw-rw-\n", v.Env().RemoteHost.MustExecute(fmt.Sprintf(`stat -c "%A" %s`, locksDir)))
	require.Equal(v.T(), "drwxr-xr-x\n", v.Env().RemoteHost.MustExecute(fmt.Sprintf(`stat -c "%A" %s`, packagesDir)))
	require.Equal(v.T(), "drwxr-xr-x\n", v.Env().RemoteHost.MustExecute(fmt.Sprintf(`stat -c "%A" %s`, installDir)))
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

func (v *vmUpdaterSuite) TestPurgeAndInstall() {
	v.Env().RemoteHost.MustExecute("sudo /opt/datadog/updater/bin/updater/updater purge")
	stableUnits := []string{
		"datadog-agent.service",
		"datadog-agent-trace.service",
		"datadog-agent-process.service",
		"datadog-agent-sysprobe.service",
		"datadog-agent-security.service",
	}
	for _, unit := range stableUnits {
		_, err := v.Env().RemoteHost.Execute(fmt.Sprintf(`systemctl is-enabled %s`, unit))
		require.Equal(
			v.T(),
			fmt.Sprintf("Failed to get unit file state for %s: No such file or directory\n: Process exited with status 1", unit),
			err.Error(),
		)
	}
	v.Env().RemoteHost.MustExecute("sudo /opt/datadog/updater/bin/updater/updater bootstrap -P datadog-agent")

	for _, unit := range stableUnits {
		require.Equal(v.T(), "enabled\n", v.Env().RemoteHost.MustExecute(fmt.Sprintf(`systemctl is-enabled %s`, unit)))
	}
	require.Equal(v.T(), "1\n", v.Env().RemoteHost.MustExecute(`sudo ls -l /opt/datadog-packages/datadog-agent/ | awk '$9 != "stable" && $3 == "dd-agent" && $4 == "dd-agent"' | wc -l`))
}
