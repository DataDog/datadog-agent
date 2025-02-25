// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package process

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
)

type windowsTestSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestWindowsTestSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &windowsTestSuite{},
		e2e.WithProvisioner(
			awshost.Provisioner(
				awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)),
				awshost.WithAgentOptions(agentparams.WithAgentConfig(processCheckConfigStr)),
			),
		),
	)
}

func (s *windowsTestSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	// Install chocolatey - https://chocolatey.org/install
	s.Env().RemoteHost.MustExecute("Set-ExecutionPolicy Bypass -Scope CurrentUser -Force; [System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072; iex ((New-Object System.Net.WebClient).DownloadString('https://community.chocolatey.org/install.ps1'))")
	// Install diskspd for IO tests - https://learn.microsoft.com/en-us/azure/azure-local/manage/diskspd-overview
	s.Env().RemoteHost.MustExecute("choco install -y diskspd")
}

func assertProcessCheck(t *testing.T, env *environments.Host) {
	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
		assertRunningChecks(collect, env.Agent.Client, []string{"process", "rtprocess"}, false)
	}, 1*time.Minute, 5*time.Second)

	var payloads []*aggregator.ProcessPayload
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		var err error
		payloads, err = env.FakeIntake.Client().GetProcesses()
		assert.NoError(c, err, "failed to get process payloads from fakeintake")

		// Wait for two payloads, as processes must be detected in two check runs to be returned
		assert.GreaterOrEqual(c, len(payloads), 2, "fewer than 2 payloads returned")
	}, 2*time.Minute, 10*time.Second)

	assertProcessCollected(t, payloads, false, "MsMpEng.exe")
}

func (s *windowsTestSuite) TestProcessCheck() {
	assertProcessCheck(s.T(), s.Env())
}

func (s *windowsTestSuite) TestProcessChecksInCoreAgent() {
	t := s.T()
	s.UpdateEnv(awshost.Provisioner(awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)),
		awshost.WithAgentOptions(agentparams.WithAgentConfig(processCheckInCoreAgentConfigStr))))
	assertProcessCheck(t, s.Env())

	// Verify the process component is not running in the core agent
	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
		status := getAgentStatus(collect, s.Env().Agent.Client)
		assert.Empty(t, status.ProcessComponentStatus.Expvars.Map.EnabledChecks, []string{})
	}, 1*time.Minute, 5*time.Second)
}

func (s *windowsTestSuite) TestProcessDiscoveryCheck() {
	t := s.T()
	s.UpdateEnv(awshost.Provisioner(
		awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)),
		awshost.WithAgentOptions(agentparams.WithAgentConfig(processDiscoveryCheckConfigStr)),
	))

	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
		assertRunningChecks(collect, s.Env().Agent.Client, []string{"process_discovery"}, false)
	}, 1*time.Minute, 5*time.Second)

	var payloads []*aggregator.ProcessDiscoveryPayload
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		var err error
		payloads, err = s.Env().FakeIntake.Client().GetProcessDiscoveries()
		assert.NoError(c, err, "failed to get process discovery payloads from fakeintake")
		assert.NotEmpty(c, payloads, "no process discovery payloads returned")
	}, 2*time.Minute, 10*time.Second)

	assertProcessDiscoveryCollected(t, payloads, "MsMpEng.exe")
}

func (s *windowsTestSuite) TestProcessCheckIO() {
	t := s.T()
	// https://datadoghq.atlassian.net/browse/CTK-3960
	flake.Mark(t)
	s.UpdateEnv(awshost.Provisioner(
		awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)),
		awshost.WithAgentOptions(agentparams.WithAgentConfig(processCheckConfigStr), agentparams.WithSystemProbeConfig(systemProbeConfigStr)),
	))

	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
		assertRunningChecks(collect, s.Env().Agent.Client, []string{"process", "rtprocess"}, true)
	}, 1*time.Minute, 5*time.Second)

	err := runDiskSpd(s.T(), s.Env().RemoteHost)
	require.NoError(s.T(), err)

	var payloads []*aggregator.ProcessPayload
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		var err error
		payloads, err = s.Env().FakeIntake.Client().GetProcesses()
		assert.NoError(c, err, "failed to get process payloads from fakeintake")

		assertProcessCollectedNew(c, payloads, true, "diskspd.exe")
	}, 2*time.Minute, 10*time.Second)
}

func (s *windowsTestSuite) TestManualProcessCheck() {
	check := s.Env().RemoteHost.
		MustExecute("& \"C:\\Program Files\\Datadog\\Datadog Agent\\bin\\agent\\process-agent.exe\" check process --json")

	assertManualProcessCheck(s.T(), check, false, "MsMpEng.exe")
}

func (s *windowsTestSuite) TestManualProcessDiscoveryCheck() {
	check := s.Env().RemoteHost.
		MustExecute("& \"C:\\Program Files\\Datadog\\Datadog Agent\\bin\\agent\\process-agent.exe\" check process_discovery --json")
	assertManualProcessDiscoveryCheck(s.T(), check, "MsMpEng.exe")
}

func (s *windowsTestSuite) TestManualProcessCheckWithIO() {
	// MsMpEng.exe process missing IO stats, agent process does not always have CPU stats populated as it is restarted multiple times during the test suite run
	// Investigation & fix tracked in https://datadoghq.atlassian.net/browse/PROCS-3757
	flake.Mark(s.T())

	s.UpdateEnv(awshost.Provisioner(
		awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)),
		awshost.WithAgentOptions(agentparams.WithAgentConfig(processCheckConfigStr), agentparams.WithSystemProbeConfig(systemProbeConfigStr)),
	))

	err := runDiskSpd(s.T(), s.Env().RemoteHost)
	require.NoError(s.T(), err)

	check := s.Env().RemoteHost.
		MustExecute("& \"C:\\Program Files\\Datadog\\Datadog Agent\\bin\\agent\\process-agent.exe\" check process -w5s --json")
	assertManualProcessCheck(s.T(), check, true, "diskspd.exe")
}

// Runs Diskspd in another ssh session
// https://github.com/Microsoft/diskspd/wiki/Command-line-and-parameters
func runDiskSpd(t *testing.T, remoteHost *components.RemoteHost) error {
	// Disk speed parameters
	// -d120: Duration of the test in seconds
	// -c128M: Size of the test file in bytes
	// -t2: Number of threads
	// -o4: Number of outstanding I/O requests per thread
	// -b8k: Block size in bytes
	// -L: Use large pages
	// -r: Random I/O
	// -Sh: Disable both software caching and hardware write caching.
	// -w50: Write percentage
	session, stdin, _, err := remoteHost.Start("diskspd -d120 -c128M -t2 -o4 -b8k -L -r -Sh -w50 disk-speed-test.dat")
	if err != nil {
		return err
	}

	t.Cleanup(func() {
		session.Close()
		stdin.Close()
	})
	return nil
}
