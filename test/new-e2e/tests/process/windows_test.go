// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package process

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	agentmodel "github.com/DataDog/agent-payload/v5/process"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-configuration/secretsutils"
)

const (
	mspmEngComm = "Antimalware Service Executable"
	diskspdComm = "DiskSpd Storage Performance Tool"
)

type windowsTestSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestWindowsTestSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &windowsTestSuite{},
		e2e.WithProvisioner(
			awshost.Provisioner(
				awshost.WithRunOptions(
					ec2.WithEC2InstanceOptions(ec2.WithOS(os.WindowsServerDefault)),
					ec2.WithAgentOptions(agentparams.WithAgentConfig(processCheckConfigStr)),
				),
			),
		),
	)
}

func (s *windowsTestSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	// SetupSuite needs to defer CleanupOnSetupFailure() if what comes after BaseSuite.SetupSuite() can fail.
	defer s.CleanupOnSetupFailure()

	// Start an antivirus scan to use as process for testing
	s.Env().RemoteHost.MustExecute("Start-MpScan -ScanType FullScan -AsJob")
	// Install chocolatey - https://chocolatey.org/install
	// This may be due to choco rate limits - https://datadoghq.atlassian.net/browse/ADXT-950
	stdout, err := s.Env().RemoteHost.Execute("Set-ExecutionPolicy Bypass -Scope Process -Force; [System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072; iwr https://community.chocolatey.org/install.ps1 -UseBasicParsing | iex")
	require.NoErrorf(s.T(), err, "Failed to install chocolatey: %s, err: %s", stdout, err)
	// Install diskspd for IO tests - https://learn.microsoft.com/en-us/azure/azure-local/manage/diskspd-overview
	stdout, err = s.Env().RemoteHost.Execute("C:\\ProgramData\\chocolatey\\bin\\choco.exe install -y diskspd")
	require.NoErrorf(s.T(), err, "Failed to install diskspd: %s, err: %s", stdout, err)
}

func (s *windowsTestSuite) TestAPIKeyRefresh() {
	t := s.T()

	secretClient := secretsutils.NewClient(t, s.Env().RemoteHost, `C:\TestFolder`)
	secretClient.SetSecret("api_key", "abcdefghijklmnopqrstuvwxyz123456")

	agentParams := []func(*agentparams.Params) error{
		agentparams.WithSkipAPIKeyInConfig(),
		agentparams.WithAgentConfig(processAgentWinRefreshStr),
	}
	agentParams = append(agentParams, secretsutils.WithWindowsSetupScript("C:/TestFolder/wrapper.bat", true)...)

	s.UpdateEnv(
		awshost.Provisioner(
			awshost.WithRunOptions(
				ec2.WithEC2InstanceOptions(ec2.WithOS(os.WindowsServerDefault)),
				ec2.WithAgentOptions(
					agentParams...,
				),
			),
		),
	)

	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
		assertAPIKeyStatus(collect, "abcdefghijklmnopqrstuvwxyz123456", s.Env().Agent.Client, false)
		assertLastPayloadAPIKey(collect, "abcdefghijklmnopqrstuvwxyz123456", s.Env().FakeIntake.Client())
	}, 2*time.Minute, 10*time.Second)

	// API key refresh
	secretClient.SetSecret("api_key", "123456abcdefghijklmnopqrstuvwxyz")
	secretRefreshOutput := s.Env().Agent.Client.Secret(agentclient.WithArgs([]string{"refresh"}))
	require.Contains(t, secretRefreshOutput, "api_key")

	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
		assertAPIKeyStatus(collect, "123456abcdefghijklmnopqrstuvwxyz", s.Env().Agent.Client, false)
		assertLastPayloadAPIKey(collect, "123456abcdefghijklmnopqrstuvwxyz", s.Env().FakeIntake.Client())
	}, 2*time.Minute, 10*time.Second)
}

func (s *windowsTestSuite) TestAPIKeyRefreshAdditionalEndpoints() {
	t := s.T()

	fakeIntakeURL := s.Env().FakeIntake.Client().URL()

	additionalEndpoint := fmt.Sprintf(`  additional_endpoints:
    "%s":
      - ENC[api_key_additional]`, fakeIntakeURL)
	config := processAgentWinRefreshStr + additionalEndpoint

	secretClient := secretsutils.NewClient(t, s.Env().RemoteHost, `C:\TestFolder`)
	apiKey := "apikeyabcde"
	apiKeyAdditional := "apikey12345"
	secretClient.SetSecret("api_key", apiKey)
	secretClient.SetSecret("api_key_additional", apiKeyAdditional)

	agentParams := []func(*agentparams.Params) error{
		agentparams.WithSkipAPIKeyInConfig(),
		agentparams.WithAgentConfig(config),
	}
	agentParams = append(agentParams, secretsutils.WithWindowsSetupScript("C:/TestFolder/wrapper.bat", true)...)

	s.UpdateEnv(
		awshost.Provisioner(
			awshost.WithRunOptions(
				ec2.WithEC2InstanceOptions(ec2.WithOS(os.WindowsServerDefault)),
				ec2.WithAgentOptions(
					agentParams...,
				),
			),
		),
	)

	fakeIntakeClient := s.Env().FakeIntake.Client()
	agentClient := s.Env().Agent.Client

	fakeIntakeClient.FlushServerAndResetAggregators()

	// Assert that the status and payloads have the correct API key
	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
		assertAPIKeyStatus(collect, apiKey, agentClient, false)
		assertAPIKeyStatus(collect, apiKeyAdditional, agentClient, false)
		assertAllPayloadsAPIKeys(collect, []string{apiKey, apiKeyAdditional}, fakeIntakeClient)
	}, 2*time.Minute, 10*time.Second)

	// Refresh secrets in the agent
	apiKey = "apikeyfghijk"
	apiKeyAdditional = "apikey67890"
	secretClient.SetSecret("api_key", apiKey)
	secretClient.SetSecret("api_key_additional", apiKeyAdditional)
	secretRefreshOutput := s.Env().Agent.Client.Secret(agentclient.WithArgs([]string{"refresh"}))
	require.Contains(t, secretRefreshOutput, "api_key")
	require.Contains(t, secretRefreshOutput, "api_key_additional")

	fakeIntakeClient.FlushServerAndResetAggregators()

	// Assert that the status and payloads have the correct API key
	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
		assertAPIKeyStatus(collect, apiKey, agentClient, false)
		assertAPIKeyStatus(collect, apiKeyAdditional, agentClient, false)
		assertAllPayloadsAPIKeys(collect, []string{apiKey, apiKeyAdditional}, fakeIntakeClient)
	}, 2*time.Minute, 10*time.Second)
}

func assertProcessCheck(t *testing.T, env *environments.Host, withIOStats bool, withSystemProbe bool, processName string, processCMDArgs []string, expectedComm string) {
	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
		assertRunningChecks(collect, env.Agent.Client, []string{"process", "rtprocess"}, withSystemProbe)
	}, 1*time.Minute, 5*time.Second)

	var payloads []*aggregator.ProcessPayload
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		var err error
		payloads, err = env.FakeIntake.Client().GetProcesses()
		assert.NoError(c, err, "failed to get process payloads from fakeintake")

		assertProcessCollectedNew(c, payloads, withIOStats, processName)

		procs := FilterProcessPayloadsByName(payloads, processName)
		require.NotEmpty(t, procs, "'%s' process not found in payloads: \n%+v", processName, payloads)
		assertProcessCommandLineArgs(c, procs, processCMDArgs)
		if expectedComm != "" {
			assertCommProperty(c, procs, expectedComm)
		}
	}, 2*time.Minute, 10*time.Second)
}

func (s *windowsTestSuite) TestProtectedProcessCheck() {
	s.UpdateEnv(awshost.Provisioner(
		awshost.WithRunOptions(
			ec2.WithEC2InstanceOptions(ec2.WithOS(os.WindowsServerDefault)),
			ec2.WithAgentOptions(agentparams.WithAgentConfig(processCheckConfigStr)),
		),
	))
	// MsMpEng.exe is a protected process so we can't access any command line arguments
	assertProcessCheck(s.T(), s.Env(), false, false, "MsMpEng.exe", []string{"MsMpEng.exe"}, mspmEngComm)
}

func (s *windowsTestSuite) TestProtectedProcessChecksInCoreAgent() {
	t := s.T()
	s.UpdateEnv(awshost.Provisioner(
		awshost.WithRunOptions(
			ec2.WithEC2InstanceOptions(ec2.WithOS(os.WindowsServerDefault)),
			ec2.WithAgentOptions(agentparams.WithAgentConfig(processCheckInCoreAgentConfigStr)),
		),
	))
	// MsMpEng.exe is a protected process so we can't access any command line arguments
	assertProcessCheck(t, s.Env(), false, false, "MsMpEng.exe", []string{"MsMpEng.exe"}, mspmEngComm)

	// Verify the process component is not running in the core agent
	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
		status := getAgentStatus(collect, s.Env().Agent.Client)
		assert.Empty(t, status.ProcessComponentStatus.Expvars.Map.EnabledChecks, []string{})
	}, 1*time.Minute, 5*time.Second)
}

func (s *windowsTestSuite) TestProcessDiscoveryCheck() {
	t := s.T()
	s.UpdateEnv(awshost.Provisioner(
		awshost.WithRunOptions(
			ec2.WithEC2InstanceOptions(ec2.WithOS(os.WindowsServerDefault)),
			ec2.WithAgentOptions(agentparams.WithAgentConfig(processDiscoveryCheckConfigStr)),
		),
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

func (s *windowsTestSuite) TestUnprotectedProcessCheckIO() {
	s.UpdateEnv(awshost.Provisioner(
		awshost.WithRunOptions(
			ec2.WithEC2InstanceOptions(ec2.WithOS(os.WindowsServerDefault)),
			ec2.WithAgentOptions(agentparams.WithAgentConfig(processCheckConfigStr), agentparams.WithSystemProbeConfig(systemProbeConfigStr)),
		),
	))

	// Flush fake intake to remove payloads that won't have IO stats
	s.Env().FakeIntake.Client().FlushServerAndResetAggregators()

	process, cmd, comm, err := runDiskSpd(s.T(), s.Env().RemoteHost)
	require.NoError(s.T(), err)

	assertProcessCheck(s.T(), s.Env(), true, true, process, cmd, comm)
}

func (s *windowsTestSuite) TestManualProcessCheck() {
	// test can be flaky due to missing CPU stats when cpu usage is extremely low (json output omits 0 values), so we want to re-run a full scan to ensure we have CPU stats
	s.Env().RemoteHost.MustExecute("Start-MpScan -ScanType FullScan -AsJob")
	check := s.Env().RemoteHost.
		MustExecute("& \"C:\\Program Files\\Datadog\\Datadog Agent\\bin\\agent\\process-agent.exe\" check process --json")
	assertManualProcessCheck(s.T(), check, false, "MsMpEng.exe")
}

func (s *windowsTestSuite) TestManualProcessDiscoveryCheck() {
	check := s.Env().RemoteHost.
		MustExecute("& \"C:\\Program Files\\Datadog\\Datadog Agent\\bin\\agent\\process-agent.exe\" check process_discovery --json")
	assertManualProcessDiscoveryCheck(s.T(), check, "MsMpEng.exe")
}

func (s *windowsTestSuite) TestManualUnprotectedProcessCheckWithIO() {
	s.UpdateEnv(awshost.Provisioner(
		awshost.WithRunOptions(
			ec2.WithEC2InstanceOptions(ec2.WithOS(os.WindowsServerDefault)),
			ec2.WithAgentOptions(agentparams.WithAgentConfig(processCheckConfigStr), agentparams.WithSystemProbeConfig(systemProbeConfigStr)),
		),
	))

	process, cmd, comm, err := runDiskSpd(s.T(), s.Env().RemoteHost)
	require.NoError(s.T(), err)

	// Try multiple times as all the I/O data may not be available in a given instant
	assert.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		check := s.Env().RemoteHost.
			MustExecute("& \"C:\\Program Files\\Datadog\\Datadog Agent\\bin\\agent\\process-agent.exe\" check process --json")
		assertManualProcessCheck(c, check, true, process)

		var checkOutput struct {
			Processes []*agentmodel.Process `json:"processes"`
		}

		err := json.Unmarshal([]byte(check), &checkOutput)
		require.NoError(c, err, "failed to unmarshal process check output")

		procs := filterProcesses(process, checkOutput.Processes)
		require.NotEmpty(c, procs, "'%s' process not found in check:\n%s\n", process, check)
		assertProcessCommandLineArgs(c, procs, cmd)
		assertCommProperty(c, procs, comm)
	}, 1*time.Minute, 5*time.Second)
}

// Runs Diskspd in another ssh session
// https://github.com/Microsoft/diskspd/wiki/Command-line-and-parameters
// diskspd is an unprotected process, so we can capture the command line
func runDiskSpd(t *testing.T, remoteHost *components.RemoteHost) (string, []string, string, error) {
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
	cmd := []string{
		"diskspd",
		"-d120",
		"-c128M",
		"-t2",
		"-o4",
		"-b8k",
		"-L",
		"-r",
		"-Sh",
		"-w50",
		"disk-speed-test.dat",
	}
	processName, err := runWindowsCommand(t, remoteHost, cmd)
	return processName, cmd, diskspdComm, err
}

func runWindowsCommand(t *testing.T, remoteHost *components.RemoteHost, cmd []string) (string, error) {
	session, stdin, _, err := remoteHost.Start(strings.Join(cmd, " "))
	if err != nil {
		return "", err
	}

	t.Cleanup(func() {
		_ = session.Close()
		_ = stdin.Close()
	})
	return cmd[0] + ".exe", nil
}
