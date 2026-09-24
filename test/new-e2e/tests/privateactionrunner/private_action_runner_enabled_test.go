// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunner

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/rshell/privilegedhelper"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	e2efakeintake "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/process"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	privateActionRunnerStartedLogLine     = "Private action runner starting"
	privateActionRunnerKeysManagerLogLine = "Keys manager ready"

	privilegedRshellBinary         = "/opt/datadog-agent/embedded/bin/rshell"
	privilegedRshellPolicy         = "/etc/datadog-agent-rshell/policy.json"
	privilegedRshellPolicyStage    = "/tmp/par-rshell-e2e-policy.json"
	privilegedRshellSocket         = "/run/datadog/rshell-privileged.sock"
	privilegedRshellSocketUnit     = "datadog-agent-rshell-privileged.socket"
	privilegedRshellServiceUnit    = "datadog-agent-rshell-privileged.service"
	privilegedRshellFixtureDir     = "/root/par-rshell-e2e"
	privilegedRshellFixture        = privilegedRshellFixtureDir + "/secret.txt"
	privilegedRshellSecret         = "privileged-rshell-e2e-secret"
	privilegedRshellKeyID          = "privileged-rshell-e2e-key"
	privateActionRunnerConfigStage = "/tmp/private-action-runner-e2e-datadog.yaml"
)

func generateTestPrivateActionRunnerConfig(t *testing.T) string {
	urn, privateKey := GenerateTestRunnerIdentity(t)
	return fmt.Sprintf(`private_action_runner:
  enabled: true
  self_enroll: false
  urn: %s
  private_key: %s
  actions_allowlist:
    - %s
  restricted_shell:
    privileged:
      enabled: true
`, urn, privateKey, runCommandAction)
}

type linuxPrivateActionRunnerEnabledSuite struct {
	e2e.BaseSuite[environments.Host]

	privilegedSigningKey testSigningKey
}

func TestLinuxPrivateActionRunnerEnabledSuite(t *testing.T) {
	t.Parallel()
	config := generateTestPrivateActionRunnerConfig(t)
	suite := &linuxPrivateActionRunnerEnabledSuite{
		privilegedSigningKey: generateTestSigningKey(t, privilegedRshellKeyID),
	}
	e2e.Run(t, suite, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(
				scenec2.WithEC2InstanceOptions(scenec2.WithOS(e2eos.Ubuntu2404E2E)),
				scenec2.WithPreAgentInstallHook(stagePrivateActionRunnerConfig(config)),
				scenec2.WithAgentOptions(
					agentparams.WithAgentConfig(config),
					agentparams.WithFile("/etc/datadog-agent/environment", "DD_INTERNAL_PAR_USE_DD_URL_FOR_OPMS=true\n", true),
				),
			),
		),
	))
}

func stagePrivateActionRunnerConfig(configContent string) func(*aws.Environment, *remote.Host) (pulumi.Resource, error) {
	return func(_ *aws.Environment, host *remote.Host) (pulumi.Resource, error) {
		staged, err := host.OS.FileManager().CopyInlineFile(pulumi.String(configContent), privateActionRunnerConfigStage)
		if err != nil {
			return nil, err
		}
		return host.OS.Runner().Command("install-private-action-runner-config", &command.Args{
			Create: pulumi.String("sudo install -d -o root -g root -m 0755 /etc/datadog-agent && sudo install -o root -g root -m 0640 " + privateActionRunnerConfigStage + " /etc/datadog-agent/datadog.yaml"),
		}, utils.PulumiDependsOn(staged))
	}
}

// TestPrivilegedRshellEndToEnd verifies the complete deployed privilege boundary:
// package permissions, systemd socket activation, TUF-authenticated task signing,
// selective elevation from dd-agent to root, and helper idle reactivation.
func (s *linuxPrivateActionRunnerEnabledSuite) TestPrivilegedRshellEndToEnd() {
	host := s.Env().RemoteHost
	client := s.Env().FakeIntake.Client()
	svcManager := common.GetServiceManager(host)
	s.Require().NotNil(svcManager)

	s.Require().Equal("root:root 755", strings.TrimSpace(host.MustExecute(
		"sudo stat -c '%U:%G %a' "+privilegedRshellBinary,
	)))
	s.Require().Equal("enabled", strings.TrimSpace(host.MustExecute(
		"sudo systemctl is-enabled "+privilegedRshellSocketUnit,
	)))
	s.Require().Equal("active", strings.TrimSpace(host.MustExecute(
		"sudo systemctl is-active "+privilegedRshellSocketUnit,
	)))
	s.Require().Equal("dd-agent:dd-agent 600", strings.TrimSpace(host.MustExecute(
		"sudo stat -c '%U:%G %a' "+privilegedRshellSocket,
	)))

	// A same-host retry may find the helper inside its idle window.
	_, err := host.Execute("sudo systemctl stop " + privilegedRshellServiceUnit)
	s.Require().NoError(err)
	s.waitForSystemdUnitState(privilegedRshellServiceUnit, "inactive", 30*time.Second)

	s.installPrivilegedRshellFixture()
	s.T().Cleanup(func() {
		_, _ = host.Execute("sudo systemctl stop " + privilegedRshellServiceUnit)
		_, _ = host.Execute("sudo rm -f " + privilegedRshellPolicy)
		_, _ = host.Execute("sudo rm -f " + privilegedRshellFixture)
		_, _ = host.Execute("sudo rmdir " + privilegedRshellFixtureDir)
	})

	// Establish that the fixture is genuinely inaccessible to PAR's service user.
	_, err = host.Execute("sudo -u dd-agent cat " + privilegedRshellFixture)
	s.Require().Error(err, "dd-agent unexpectedly read the root-only fixture")

	_, err = svcManager.Start(privateActionRunnerServiceName)
	s.Require().NoError(err)
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		status, statusErr := svcManager.Status(privateActionRunnerServiceName)
		require.NoError(c, statusErr)
		require.Contains(c, status, "active")
	}, 2*time.Minute, 5*time.Second)

	s.Require().NoError(client.FlushPAR())
	s.deleteRCConfig(runnerKeysRCProduct, privilegedRshellKeyID)
	s.T().Cleanup(func() { s.deleteRCConfig(runnerKeysRCProduct, privilegedRshellKeyID) })

	stats, err := client.RCStats()
	s.Require().NoError(err)
	s.Require().NoError(client.RCAddConfig(
		strconv.FormatInt(testRunnerOrgID, 10),
		runnerKeysRCProduct,
		s.privilegedSigningKey.id,
		s.privilegedSigningKey.id,
		s.privilegedSigningKey.config,
	))
	setPARTaskSigningKey(s.T(), client, s.privilegedSigningKey)

	// Wait for the Core Agent to fetch the TUF target after it was added. Two
	// polls avoid racing the request which was already in flight at RCAddConfig.
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		current, statsErr := client.RCStats()
		require.NoError(c, statsErr)
		require.GreaterOrEqual(c, current.Polls, stats.Polls+2)
	}, 45*time.Second, time.Second)

	nonElevated := s.runPrivilegedRshellTask("cat " + privilegedRshellFixture)
	s.Require().True(nonElevated.Success, "non-elevated rshell command should complete: %+v", nonElevated)
	s.Require().NotZero(rshellExitCode(s.T(), nonElevated))
	s.Require().NotContains(nonElevated.Outputs["stdout"], privilegedRshellSecret)

	privilegedTaskID, elevated := s.runPrivilegedRshellTaskWithID("sudo cat " + privilegedRshellFixture)
	s.Require().True(elevated.Success, "privileged rshell command failed: %+v", elevated)
	s.Require().Zero(rshellExitCode(s.T(), elevated), "privileged rshell command failed: %+v", elevated)
	s.Require().Equal(privilegedRshellSecret+"\n", elevated.Outputs["stdout"])
	s.Require().Equal("", elevated.Outputs["stderr"])

	s.waitForSystemdUnitState(privilegedRshellServiceUnit, "active", 20*time.Second)
	s.assertPrivilegedHelperUIDs()
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		host.MustExecuteOn(c, fmt.Sprintf(
			"sudo journalctl -u %s --no-pager --grep %q",
			privilegedRshellServiceUnit,
			privilegedTaskID,
		))
	}, 20*time.Second, time.Second, "helper journal should identify the authorized task")

	// The root helper exits when idle, while its socket remains available for
	// the next task. A second success proves socket reactivation works.
	s.waitForSystemdUnitState(privilegedRshellServiceUnit, "inactive", 50*time.Second)
	s.Require().Equal("active", strings.TrimSpace(host.MustExecute(
		"sudo systemctl is-active "+privilegedRshellSocketUnit,
	)))
	reactivated := s.runPrivilegedRshellTask("sudo cat " + privilegedRshellFixture)
	s.Require().True(reactivated.Success, "reactivated privileged helper failed: %+v", reactivated)
	s.Require().Zero(rshellExitCode(s.T(), reactivated), "reactivated privileged helper failed: %+v", reactivated)
	s.Require().Equal(privilegedRshellSecret+"\n", reactivated.Outputs["stdout"])
}

func (s *linuxPrivateActionRunnerEnabledSuite) installPrivilegedRshellFixture() {
	rootJSON, err := e2efakeintake.RCRootJSON()
	s.Require().NoError(err)
	policy, err := json.Marshal(struct {
		Version            int             `json:"version"`
		OrgID              int64           `json:"orgId"`
		RunnerID           string          `json:"runnerId"`
		DirectorRoot       json.RawMessage `json:"directorRoot"`
		AllowedCommands    []string        `json:"allowedCommands"`
		AllowedPaths       []string        `json:"allowedPaths"`
		ElevatableCommands []string        `json:"elevatableCommands"`
	}{
		Version:            privilegedhelper.ProtocolVersion,
		OrgID:              testRunnerOrgID,
		RunnerID:           testRunnerRunnerID,
		DirectorRoot:       json.RawMessage(rootJSON),
		AllowedCommands:    []string{"rshell:cat"},
		AllowedPaths:       []string{privilegedRshellFixtureDir + ":ro"},
		ElevatableCommands: []string{"rshell:cat"},
	})
	s.Require().NoError(err)

	host := s.Env().RemoteHost
	_, err = host.WriteFile(privilegedRshellPolicyStage, policy)
	s.Require().NoError(err)
	_, err = host.WriteFile(privilegedRshellPolicyStage+".secret", []byte(privilegedRshellSecret+"\n"))
	s.Require().NoError(err)
	host.MustExecute("sudo install -d -o root -g root -m 0700 " + privilegedRshellFixtureDir)
	host.MustExecute("sudo install -o root -g root -m 0600 " + privilegedRshellPolicyStage + ".secret " + privilegedRshellFixture)
	host.MustExecute("sudo install -o root -g root -m 0600 " + privilegedRshellPolicyStage + " " + privilegedRshellPolicy)
	_, _ = host.Execute("rm -f " + privilegedRshellPolicyStage)
	_, _ = host.Execute("rm -f " + privilegedRshellPolicyStage + ".secret")
}

func (s *linuxPrivateActionRunnerEnabledSuite) runPrivilegedRshellTask(command string) *api.PARTaskResult {
	_, result := s.runPrivilegedRshellTaskWithID(command)
	return result
}

func (s *linuxPrivateActionRunnerEnabledSuite) runPrivilegedRshellTaskWithID(command string) (string, *api.PARTaskResult) {
	taskID := uuid.New().String()
	err := s.Env().FakeIntake.Client().EnqueuePARTask(taskID, runCommandAction, map[string]interface{}{
		"command":              command,
		"effectivePermissions": privilegedhelper.EscalationAllowed,
		"elevatableCommands":   []string{"rshell:cat"},
		"allowedCommands":      []string{"rshell:cat"},
		"allowedPaths":         []string{privilegedRshellFixtureDir + ":ro"},
	})
	s.Require().NoError(err)
	result, err := s.Env().FakeIntake.Client().GetPARTaskResult(taskID, 2*time.Minute)
	s.Require().NoError(err)
	return taskID, result
}

func (s *linuxPrivateActionRunnerEnabledSuite) deleteRCConfig(product, configID string) {
	configs, err := s.Env().FakeIntake.Client().RCListConfigs()
	s.Require().NoError(err)
	for _, config := range configs {
		if config.Product == product && config.ConfigID == configID {
			key := fmt.Sprintf("%s/%s/%s/%s", config.OrgID, config.Product, config.ConfigID, config.ConfigName)
			s.Require().NoError(s.Env().FakeIntake.Client().RCDeleteConfig(key))
		}
	}
}

func (s *linuxPrivateActionRunnerEnabledSuite) waitForSystemdUnitState(unit, state string, timeout time.Duration) {
	s.T().Helper()
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		output, err := s.Env().RemoteHost.Execute("sudo systemctl is-active " + unit + " || true")
		require.NoError(c, err)
		require.Equal(c, state, strings.TrimSpace(output))
	}, timeout, time.Second, "%s should become %s", unit, state)
}

func (s *linuxPrivateActionRunnerEnabledSuite) assertPrivilegedHelperUIDs() {
	host := s.Env().RemoteHost
	pid := strings.TrimSpace(host.MustExecute(
		"sudo systemctl show --property MainPID --value " + privilegedRshellServiceUnit,
	))
	s.Require().NotEqual("0", pid)
	ddAgentUID := strings.TrimSpace(host.MustExecute("id -u dd-agent"))
	status := host.MustExecute("sudo cat /proc/" + pid + "/status")
	for _, line := range strings.Split(status, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 5 && fields[0] == "Uid:" {
			s.Require().Equal("0", fields[1], "helper must retain real UID 0")
			s.Require().Equal(ddAgentUID, fields[2], "helper must serve requests with dd-agent effective UID")
			return
		}
	}
	s.FailNow("helper process status has no Uid line: %s", status)
}

func (s *linuxPrivateActionRunnerEnabledSuite) TestPrivateActionRunnerStartsWhenEnabled() {
	host := s.Env().RemoteHost
	svcManager := common.GetServiceManager(host)
	s.Require().NotNil(svcManager)

	// Start the private action runner service
	_, err := svcManager.Start(privateActionRunnerServiceName)
	s.Require().NoError(err)

	// Verify the service is running
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		status, statusErr := svcManager.Status(privateActionRunnerServiceName)
		assert.NoError(c, statusErr)
		assert.Contains(c, status, "active")
	}, 2*time.Minute, 5*time.Second, "private action runner service should be active when enabled")

	// Verify the process is running
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		pids, pidErr := process.FindPID(host, "privateactionrunner")
		assert.NoError(c, pidErr)
		assert.NotEmpty(c, pids, "privateactionrunner process should be running")
	}, 2*time.Minute, 5*time.Second, "privateactionrunner process should be running when enabled")

	// Verify the log file exists
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		host.MustExecuteOn(c, "sudo test -f "+privateActionRunnerLogFile)
	}, 2*time.Minute, 5*time.Second, "private action runner log file should exist")

	// Verify log contains startup message
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		host.MustExecuteOn(c, fmt.Sprintf("sudo grep -i %q %s", privateActionRunnerStartedLogLine, privateActionRunnerLogFile))
	}, 2*time.Minute, 5*time.Second, "private action runner log should contain the started message")

	// Wait for the Core Agent to report the AP_RUNNER_KEYS client in its backend requests.
	client := s.Env().FakeIntake.Client()
	stats, err := client.RCStats()
	s.Require().NoError(err)
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		current, statsErr := client.RCStats()
		assert.NoError(c, statsErr)
		if statsErr == nil {
			assert.GreaterOrEqual(c, current.Polls, stats.Polls+2)
		}
	}, 45*time.Second, time.Second, "Core Agent should poll after PAR subscribes")
	PushFakeRunnerKeysConfig(s.T(), client)

	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		host.MustExecuteOn(c, fmt.Sprintf("sudo grep -F %q %s", privateActionRunnerKeysManagerLogLine, privateActionRunnerLogFile))
	}, 30*time.Second, time.Second, "private action runner log should report the keys manager ready")
}

func (s *linuxPrivateActionRunnerEnabledSuite) TestPrivateActionRunnerServiceRestart() {
	host := s.Env().RemoteHost
	svcManager := common.GetServiceManager(host)
	s.Require().NotNil(svcManager)

	PushFakeRunnerKeysConfig(s.T(), s.Env().FakeIntake.Client())

	// Ensure service is started
	_, err := svcManager.Start(privateActionRunnerServiceName)
	s.Require().NoError(err)

	// Wait for service to be running
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		status, statusErr := svcManager.Status(privateActionRunnerServiceName)
		assert.NoError(c, statusErr)
		assert.Contains(c, status, "active")
	}, 2*time.Minute, 5*time.Second)

	// Get the original PID
	var originalPID int
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		pids, pidErr := process.FindPID(host, "privateactionrunner")
		assert.NoError(c, pidErr)
		assert.NotEmpty(c, pids)
		if len(pids) > 0 {
			originalPID = pids[0]
		}
	}, 2*time.Minute, 5*time.Second)

	// Restart the service
	_, err = svcManager.Restart(privateActionRunnerServiceName)
	s.Require().NoError(err)

	// Verify service is running again
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		status, statusErr := svcManager.Status(privateActionRunnerServiceName)
		assert.NoError(c, statusErr)
		assert.Contains(c, status, "active")
	}, 2*time.Minute, 5*time.Second, "private action runner should be active after restart")

	// Verify we have a new PID (service actually restarted)
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		pids, pidErr := process.FindPID(host, "privateactionrunner")
		assert.NoError(c, pidErr)
		assert.NotEmpty(c, pids)
		if len(pids) > 0 {
			assert.NotEqual(c, originalPID, pids[0], "PID should change after restart")
		}
	}, 2*time.Minute, 5*time.Second, "privateactionrunner should have a new PID after restart")
}
