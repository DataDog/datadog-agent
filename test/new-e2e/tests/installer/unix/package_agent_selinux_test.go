// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

type packageAgentSELinuxSuite struct{ packageBaseSuite }

func TestPackageAgentSELinux(t *testing.T) {
	require.NotEmpty(t, os.Getenv("E2E_PIPELINE_ID"))
	t.Parallel()
	descriptor := e2eos.NewDescriptor(e2eos.RedHat, "86")
	s := &packageAgentSELinuxSuite{packageBaseSuite: newPackageSuite("agent-selinux", descriptor, e2eos.AMD64Arch, InstallMethodInstallScript)}
	e2e.Run(t, s, e2e.WithProvisioner(awshost.Provisioner(awshost.WithRunOptions(
		ec2.WithEC2InstanceOptions(ec2.WithOSArch(descriptor, e2eos.AMD64Arch), ec2.WithInternetAccess()),
		ec2.WithoutAgent(),
	))), e2e.WithStackName(s.Name()))
}

func (s *packageAgentSELinuxSuite) TestFleetUpgradeAndReboot() {
	remote := s.Env().RemoteHost
	require.Equal(s.T(), "8", strings.TrimSpace(remote.MustExecute(`. /etc/os-release && printf '%s' "${VERSION_ID%%.*}"`)))
	require.Equal(s.T(), "Enforcing", strings.TrimSpace(remote.MustExecute("getenforce")))
	remote.MustExecute("sudo sh -c 'command -v semanage && command -v restorecon'")
	defer s.Purge()
	defer func() {
		if !s.T().Failed() {
			return
		}
		for _, command := range []string{
			"sudo systemctl status datadog-agent datadog-agent-trace datadog-agent-installer --no-pager",
			"sudo journalctl -b -u datadog-agent -u datadog-agent-trace -u datadog-agent-installer --no-pager -n 200",
			"readlink -f /opt/datadog-packages/datadog-agent/stable",
			"sudo ls -ldZ /opt/datadog-packages/datadog-agent/stable/run; sudo ls -lZ /opt/datadog-packages/datadog-agent/stable/run/",
			"sudo datadog-installer status",
			"sudo ausearch -m AVC,USER_AVC -ts boot",
		} {
			output, err := remote.Execute(command)
			s.T().Logf("%s: %v\n%s", command, err, output)
		}
	}()
	s.RunInstallScript()
	s.host.AssertPackageInstalledByPackageManager("datadog-agent")
	baselinePath := strings.TrimSpace(remote.MustExecute("readlink -f /opt/datadog-packages/datadog-agent/stable"))
	remote.MustExecute(`printf '\nremote_updates: true\n' | sudo tee -a /etc/datadog-agent/datadog.yaml`)
	remote.MustExecute("sudo systemctl restart datadog-agent")
	s.host.WaitForUnitActive(s.T(), agentUnit, traceUnit, installerUnit)
	upgrade := &upgradeScenarioSuite{packageBaseSuite: s.packageBaseSuite}
	upgrade.setCatalog(upgrade.testCatalog())
	_, err := upgrade.startExperiment(datadogAgent, s.pipelineAgentVersion)
	require.NoError(s.T(), err)
	s.host.WaitForUnitActive(s.T(), agentUnitXP, traceUnitXP)
	_, err = upgrade.promoteExperiment(datadogAgent)
	require.NoError(s.T(), err)
	s.host.WaitForUnitActive(s.T(), agentUnit, traceUnit, installerUnit)
	s.EventuallyWithT(func(c *assert.CollectT) {
		state := upgrade.getInstallerStatus().Packages.States["datadog-agent"]
		assert.Equal(c, s.pipelineAgentVersion, state.Stable)
		assert.Empty(c, state.Experiment)
	}, 2*time.Minute, time.Second)
	upgradedPath := strings.TrimSpace(remote.MustExecute("readlink -f /opt/datadog-packages/datadog-agent/stable"))
	require.NotEqual(s.T(), baselinePath, upgradedPath)
	require.Equal(s.T(), "/opt/datadog-packages/datadog-agent/"+s.pipelineAgentVersion, upgradedPath)
	s.assertRuntimeLabels()
	before := strings.TrimSpace(remote.MustExecute("cat /proc/sys/kernel/random/boot_id"))
	if _, err := remote.Execute("sudo systemctl --no-block reboot"); err != nil {
		s.T().Logf("reboot request: %v", err)
	}
	s.EventuallyWithT(func(c *assert.CollectT) {
		require.NoError(c, remote.Reconnect())
		after, err := remote.Execute("cat /proc/sys/kernel/random/boot_id")
		require.NoError(c, err)
		require.NotEmpty(c, strings.TrimSpace(after))
		assert.NotEqual(c, before, strings.TrimSpace(after))
	}, 20*time.Minute, 15*time.Second)
	require.Equal(s.T(), "Enforcing", strings.TrimSpace(remote.MustExecute("getenforce")))
	s.host.WaitForUnitActive(s.T(), agentUnit, traceUnit, installerUnit)
	state := upgrade.getInstallerStatus().Packages.States["datadog-agent"]
	require.Equal(s.T(), s.pipelineAgentVersion, state.Stable)
	require.Empty(s.T(), state.Experiment)
	remote.MustExecute("sudo datadog-agent status")
	s.assertRuntimeLabels()
}

func (s *packageAgentSELinuxSuite) assertRuntimeLabels() {
	s.T().Helper()
	remote := s.Env().RemoteHost
	const run = "/opt/datadog-packages/datadog-agent/stable/run"
	for _, dir := range []string{run, "/opt/datadog-packages/run"} {
		require.Equal(s.T(), "dd-agent:dd-agent 755", strings.TrimSpace(remote.MustExecute("stat -Lc '%U:%G %a' "+dir)))
		s.assertRuntimeType(dir)
	}
	for _, process := range []struct{ unit, file string }{{agentUnit, "agent.pid"}, {traceUnit, "trace-agent.pid"}} {
		s.EventuallyWithT(func(c *assert.CollectT) {
			pid, err := remote.Execute("cat " + run + "/" + process.file)
			require.NoError(c, err)
			number, err := strconv.Atoi(strings.TrimSpace(pid))
			require.NoError(c, err)
			require.Positive(c, number)
			mainPID, err := remote.Execute("systemctl show -p MainPID --value " + process.unit)
			require.NoError(c, err)
			assert.Equal(c, strings.TrimSpace(mainPID), strings.TrimSpace(pid))
		}, time.Minute, time.Second)
		s.assertRuntimeType(run + "/" + process.file)
	}
	files := remote.MustExecute("find -H " + run + " -maxdepth 1 -type f -name '*.pid'")
	for _, file := range strings.Fields(files) {
		s.assertRuntimeType(file)
	}
}

func (s *packageAgentSELinuxSuite) assertRuntimeType(path string) {
	s.T().Helper()
	context := strings.TrimSpace(s.Env().RemoteHost.MustExecute(fmt.Sprintf("stat -Lc '%%C' %q", path)))
	fields := strings.Split(context, ":")
	require.GreaterOrEqual(s.T(), len(fields), 3, "missing SELinux context for %s: %s", path, context)
	require.Equal(s.T(), "var_run_t", fields[2], path)
}
