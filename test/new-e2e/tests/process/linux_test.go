// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package process

import (
	_ "embed"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-configuration/secretsutils"
)

const (
	zombieFixtureScriptPath = "/tmp/cxp-zombie-parent.py"
	zombieFixtureStateDir   = "/tmp/cxp-zombie-aggregation"
)

//go:embed fixtures/zombie_parent.py
var zombieParentScript string

type linuxTestSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestLinuxTestSuite(t *testing.T) {
	t.Parallel()
	agentParams := []func(*agentparams.Params) error{
		agentparams.WithAgentConfig(processCheckConfigStr),
	}

	options := []e2e.SuiteOption{
		e2e.WithProvisioner(awshost.Provisioner(awshost.WithRunOptions(scenec2.WithAgentOptions(agentParams...)))),
	}

	e2e.Run(t, &linuxTestSuite{}, options...)
}

func (s *linuxTestSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	// SetupSuite needs to defer CleanupOnSetupFailure() if what comes after BaseSuite.SetupSuite() can fail.
	defer s.CleanupOnSetupFailure()

	// Start a process and keep it running
	s.Env().RemoteHost.MustExecute("nohup stress -d 1 >myscript.log 2>&1 </dev/null &")
}

func (s *linuxTestSuite) TestAPIKeyRefreshCoreAgent() {
	t := s.T()

	secretClient := secretsutils.NewClient(t, s.Env().RemoteHost, "/tmp/test-secret")
	secretClient.SetSecret("api_key", "abcdefghijklmnopqrstuvwxyz123456")

	s.UpdateEnv(
		awshost.Provisioner(
			awshost.WithRunOptions(scenec2.WithAgentOptions(
				agentparams.WithAgentConfig(coreAgentRefreshStr),
				secretsutils.WithUnixSetupScript("/tmp/test-secret/secret-resolver.py", false),
				agentparams.WithSkipAPIKeyInConfig(),
			)),
		),
	)

	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
		assertAPIKeyStatus(collect, "abcdefghijklmnopqrstuvwxyz123456", s.Env().Agent.Client, true)
		assertLastPayloadAPIKey(collect, "abcdefghijklmnopqrstuvwxyz123456", s.Env().FakeIntake.Client())
	}, 2*time.Minute, 10*time.Second)

	// API key refresh
	secretClient.SetSecret("api_key", "123456abcdefghijklmnopqrstuvwxyz")
	secretRefreshOutput := s.Env().Agent.Client.Secret(agentclient.WithArgs([]string{"refresh"}))
	require.Contains(t, secretRefreshOutput, "api_key")

	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
		assertAPIKeyStatus(collect, "123456abcdefghijklmnopqrstuvwxyz", s.Env().Agent.Client, true)
		assertLastPayloadAPIKey(collect, "123456abcdefghijklmnopqrstuvwxyz", s.Env().FakeIntake.Client())
	}, 2*time.Minute, 10*time.Second)
}

func (s *linuxTestSuite) TestAPIKeyRefreshAdditionalEndpoints() {
	t := s.T()

	fakeIntakeURL := s.Env().FakeIntake.Client().URL()

	additionalEndpoint := fmt.Sprintf(`  additional_endpoints:
    "%s":
      - ENC[api_key_additional]`, fakeIntakeURL)
	config := coreAgentRefreshStr + additionalEndpoint

	secretClient := secretsutils.NewClient(t, s.Env().RemoteHost, "/tmp/test-secret")
	apiKey := "apikeyabcde"
	apiKeyAdditional := "apikey12345"
	secretClient.SetSecret("api_key", apiKey)
	secretClient.SetSecret("api_key_additional", apiKeyAdditional)

	s.UpdateEnv(
		awshost.Provisioner(
			awshost.WithRunOptions(scenec2.WithAgentOptions(
				agentparams.WithAgentConfig(config),
				secretsutils.WithUnixSetupScript("/tmp/test-secret/secret-resolver.py", false),
				agentparams.WithSkipAPIKeyInConfig(),
			)),
		),
	)

	fakeIntakeClient := s.Env().FakeIntake.Client()
	agentClient := s.Env().Agent.Client

	fakeIntakeClient.FlushServerAndResetAggregators()

	// Assert that the status and payloads have the correct API key
	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
		assertAPIKeyStatus(collect, apiKey, agentClient, true)
		assertAPIKeyStatus(collect, apiKeyAdditional, agentClient, true)
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
		assertAPIKeyStatus(collect, apiKey, agentClient, true)
		assertAPIKeyStatus(collect, apiKeyAdditional, agentClient, true)
		assertAllPayloadsAPIKeys(collect, []string{apiKey, apiKeyAdditional}, fakeIntakeClient)
	}, 2*time.Minute, 10*time.Second)
}

func (s *linuxTestSuite) TestProcessCheck() {
	t := s.T()
	s.UpdateEnv(awshost.Provisioner(awshost.WithRunOptions(scenec2.WithAgentOptions(agentparams.WithAgentConfig(processCheckConfigStr)))))

	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
		assertRunningChecks(collect, s.Env().Agent.Client, []string{"process", "rtprocess", "service_discovery"}, false)
	}, 2*time.Minute, 5*time.Second)

	var payloads []*aggregator.ProcessPayload
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		var err error
		payloads, err = s.Env().FakeIntake.Client().GetProcesses()
		assert.NoError(c, err, "failed to get process payloads from fakeintake")

		// Wait for two payloads, as processes must be detected in two check runs to be returned
		assert.GreaterOrEqual(c, len(payloads), 2, "fewer than 2 payloads returned")

		assertProcessCollected(c, payloads, false, "stress")
	}, 2*time.Minute, 10*time.Second)
}

func (s *linuxTestSuite) TestZombieProcessAggregation() {
	t := s.T()

	configForMode := func(ignoreZombies bool) string {
		return fmt.Sprintf("%s\n  ignore_zombie_processes: %t\n", strings.TrimRight(processCheckConfigStr, "\n"), ignoreZombies)
	}
	updateEnv := func(ignoreZombies bool) {
		s.UpdateEnv(awshost.Provisioner(awshost.WithRunOptions(scenec2.WithAgentOptions(
			agentparams.WithAgentConfig(configForMode(ignoreZombies)),
			agentparams.WithFile(zombieFixtureScriptPath, zombieParentScript, false),
		))))
		assert.EventuallyWithT(t, func(c *assert.CollectT) {
			assertRunningChecks(c, s.Env().Agent.Client, []string{"process", "rtprocess", "service_discovery"}, false)
		}, 2*time.Minute, 5*time.Second)
	}
	cleanupFixture := func() {
		s.Env().RemoteHost.MustExecute(fmt.Sprintf(
			"sudo sh -c 'if test -f %[1]s/parent.pid; then kill $(cat %[1]s/parent.pid) 2>/dev/null || true; fi; rm -rf %[1]s'",
			zombieFixtureStateDir,
		))
	}
	waitForPID := func(path string) int32 {
		var pid int64
		require.EventuallyWithT(t, func(c *assert.CollectT) {
			output := s.Env().RemoteHost.MustExecuteOn(c, "cat "+path)
			parsed, err := strconv.ParseInt(strings.TrimSpace(output), 10, 32)
			require.NoError(c, err, "failed to parse PID from %s: %q", path, output)
			pid = parsed
		}, time.Minute, time.Second)
		return int32(pid)
	}
	assertRealtimeExcludesPID := func(pid int32) {
		assert.EventuallyWithT(t, func(c *assert.CollectT) {
			check := s.Env().RemoteHost.MustExecuteOn(c, "sudo datadog-agent processchecks rtprocess --json")
			assertManualRTProcessNotCollected(c, check, pid)
		}, 2*time.Minute, 10*time.Second)
	}

	updateEnv(false)
	cleanupFixture()
	t.Cleanup(func() {
		if t.Failed() {
			fixtureLog, err := s.Env().RemoteHost.Execute("cat " + zombieFixtureStateDir + "/fixture.log")
			if err != nil {
				t.Logf("failed to collect zombie fixture log: %v", err)
			} else {
				t.Logf("zombie fixture log:\n%s", fixtureLog)
			}
		}
		cleanupFixture()
	})
	require.NoError(t, s.Env().FakeIntake.Client().FlushServerAndResetAggregators())
	s.Env().RemoteHost.MustExecute(fmt.Sprintf(
		"mkdir -p %[1]s && nohup python3 %[2]s %[1]s >%[1]s/fixture.log 2>&1 </dev/null &",
		zombieFixtureStateDir,
		zombieFixtureScriptPath,
	))
	parentPID := waitForPID(zombieFixtureStateDir + "/parent.pid")

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		payloads, err := s.Env().FakeIntake.Client().GetProcesses()
		require.NoError(c, err, "failed to get process payloads from fakeintake")
		assertProcessPIDCollected(c, payloads, parentPID)
	}, 2*time.Minute, 10*time.Second)

	// Start the zombie only after its parent is established in the previous
	// process collection, making the positive creation rate observable.
	require.NoError(t, s.Env().FakeIntake.Client().FlushServerAndResetAggregators())
	s.Env().RemoteHost.MustExecute("touch " + zombieFixtureStateDir + "/trigger")
	zombiePID := waitForPID(zombieFixtureStateDir + "/child.pid")
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		state := s.Env().RemoteHost.MustExecuteOn(c, fmt.Sprintf("awk '$1 == \"State:\" {print $2}' /proc/%d/status", zombiePID))
		assert.Equal(c, "Z", strings.TrimSpace(state), "fixture child %d is not a zombie", zombiePID)
	}, time.Minute, time.Second)

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		payloads, err := s.Env().FakeIntake.Client().GetProcesses()
		require.NoError(c, err, "failed to get process payloads from fakeintake")
		assertZombieAggregationPayloads(c, payloads, parentPID, zombiePID, true)
	}, 2*time.Minute, 5*time.Second)
	assertRealtimeExcludesPID(zombiePID)

	// Preserve the same fixture across the Agent restart and verify the
	// compatibility mode disables aggregation while continuing to omit zombies.
	updateEnv(true)
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		state := s.Env().RemoteHost.MustExecuteOn(c, fmt.Sprintf("awk '$1 == \"State:\" {print $2}' /proc/%d/status", zombiePID))
		assert.Equal(c, "Z", strings.TrimSpace(state), "fixture zombie %d did not survive the Agent update", zombiePID)
	}, time.Minute, time.Second)
	require.NoError(t, s.Env().FakeIntake.Client().FlushServerAndResetAggregators())

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		payloads, err := s.Env().FakeIntake.Client().GetProcesses()
		require.NoError(c, err, "failed to get process payloads from fakeintake")
		assertZombieAggregationPayloads(c, payloads, parentPID, zombiePID, false)
	}, 2*time.Minute, 10*time.Second)
	assertRealtimeExcludesPID(zombiePID)
}

func (s *linuxTestSuite) TestProcessDiscoveryCheck() {
	t := s.T()
	s.UpdateEnv(awshost.Provisioner(awshost.WithRunOptions(scenec2.WithAgentOptions(agentparams.WithAgentConfig(processDiscoveryCheckConfigStr), agentparams.WithSystemProbeConfig(discoveryDisabledConfigStr)))))

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

	assertProcessDiscoveryCollected(t, payloads, "stress")
}

func (s *linuxTestSuite) TestProcessCheckWithIO() {
	t := s.T()
	s.UpdateEnv(awshost.Provisioner(awshost.WithRunOptions(scenec2.WithAgentOptions(agentparams.WithAgentConfig(processCheckConfigStr), agentparams.WithSystemProbeConfig(systemProbeConfigStr)))))

	// Flush fake intake to remove payloads that won't have IO stats
	s.Env().FakeIntake.Client().FlushServerAndResetAggregators()

	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
		assertRunningChecks(collect, s.Env().Agent.Client, []string{"process", "rtprocess", "service_discovery"}, true)
	}, 1*time.Minute, 5*time.Second)

	var payloads []*aggregator.ProcessPayload
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		var err error
		payloads, err = s.Env().FakeIntake.Client().GetProcesses()
		assert.NoError(c, err, "failed to get process payloads from fakeintake")

		// Wait for two payloads, as processes must be detected in two check runs to be returned
		assert.GreaterOrEqual(c, len(payloads), 2, "fewer than 2 payloads returned")

		assertProcessCollected(c, payloads, true, "stress")
	}, 2*time.Minute, 10*time.Second)
}

func (s *linuxTestSuite) TestProcessChecksWithNPM() {
	t := s.T()
	s.UpdateEnv(awshost.Provisioner(awshost.WithRunOptions(scenec2.WithAgentOptions(agentparams.WithAgentConfig(processCheckConfigStr), agentparams.WithSystemProbeConfig(systemProbeNPMConfigStr)))))

	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
		assertRunningChecks(collect, s.Env().Agent.Client, []string{"process", "rtprocess", "service_discovery", "connections"}, false)
	}, 1*time.Minute, 5*time.Second)

	// Flush fake intake to remove any payloads which may have
	s.Env().FakeIntake.Client().FlushServerAndResetAggregators()

	var payloads []*aggregator.ProcessPayload
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		var err error
		payloads, err = s.Env().FakeIntake.Client().GetProcesses()
		assert.NoError(c, err, "failed to get process payloads from fakeintake")

		// Wait for two payloads, as processes must be detected in two check runs to be returned
		assert.GreaterOrEqual(c, len(payloads), 2, "fewer than 2 payloads returned")

		assertProcessCollected(c, payloads, false, "stress")
	}, 2*time.Minute, 10*time.Second)
}

func (s *linuxTestSuite) TestManualProcessCheck() {
	s.UpdateEnv(awshost.Provisioner(awshost.WithRunOptions(scenec2.WithAgentOptions(agentparams.WithAgentConfig(processCheckConfigStr)))))

	assert.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		check := s.Env().RemoteHost.MustExecuteOn(c, "sudo datadog-agent processchecks process --json")
		assertManualProcessCheck(c, check, false, "stress")
	}, 2*time.Minute, 10*time.Second)
}

func (s *linuxTestSuite) TestManualRTProcessCheckCoreAgent() {
	s.UpdateEnv(awshost.Provisioner(awshost.WithRunOptions(scenec2.WithAgentOptions(agentparams.WithAgentConfig(processCheckConfigStr)))))

	assert.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		check := s.Env().RemoteHost.MustExecuteOn(c, "sudo datadog-agent processchecks rtprocess --json")
		assertManualRTProcessCheck(c, check)
	}, 2*time.Minute, 10*time.Second)
}

func (s *linuxTestSuite) TestManualProcessDiscoveryCheck() {
	assert.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		check := s.Env().RemoteHost.MustExecuteOn(c, "sudo datadog-agent processchecks process_discovery --json")
		assertManualProcessDiscoveryCheck(c, check, "stress")
	}, 2*time.Minute, 10*time.Second)
}

func (s *linuxTestSuite) TestManualProcessCheckWithIO() {
	// https://datadoghq.atlassian.net/browse/CXP-2594
	flake.Mark(s.T())

	s.UpdateEnv(awshost.Provisioner(awshost.WithRunOptions(scenec2.WithAgentOptions(
		agentparams.WithAgentConfig(processCheckConfigStr),
		agentparams.WithSystemProbeConfig(systemProbeConfigStr)))))

	assert.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		check := s.Env().RemoteHost.MustExecuteOn(c, "sudo datadog-agent processchecks process --json")
		assertManualProcessCheck(c, check, true, "stress")
	}, 2*time.Minute, 10*time.Second)
}
