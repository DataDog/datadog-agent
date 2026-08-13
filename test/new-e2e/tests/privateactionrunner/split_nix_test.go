// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunner

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/errorcode"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	awsfakeintake "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

const (
	parControlProcess  = "datadog-agent-par-control"
	parExecutorProcess = "datadog-agent-action-executor"
	procmgrCLI         = "/opt/datadog-agent/embedded/bin/dd-procmgr"
	procmgrSocket      = "/var/run/datadog-procmgrd/dd-procmgrd.sock"
	parIdentityPath    = "/etc/datadog-agent/privateactionrunner_private_identity.json"
	parConfigStaging   = "/tmp/par-e2e-datadog.yaml"
)

type linuxPARSplitSuite struct {
	e2e.BaseSuite[environments.Host]

	baselineConfig string
	inlineURN      string
	inlineKey      string
	persistedURN   string
	persistedKey   string
	signingKey1    testSigningKey
	signingKey2    testSigningKey
}

func TestLinuxPARSplitSuite(t *testing.T) {
	t.Parallel()
	inlineURN, inlineKey := GenerateTestRunnerIdentity(t)
	persistedURN, persistedKey := GenerateTestRunnerIdentity(t)
	config := splitConfig(inlineURN, inlineKey)
	suite := &linuxPARSplitSuite{
		baselineConfig: config,
		inlineURN:      inlineURN,
		inlineKey:      inlineKey,
		persistedURN:   persistedURN,
		persistedKey:   persistedKey,
		signingKey1:    generateTestSigningKey(t, "runner-key-1"),
		signingKey2:    generateTestSigningKey(t, "runner-key-2"),
	}

	e2e.Run(t, suite, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(
				scenec2.WithFakeIntakeOptions(awsfakeintake.WithLoadBalancer()),
				scenec2.WithAgentOptions(
					agentparams.WithAgentConfig(config),
					agentparams.WithFile("/etc/datadog-agent/environment", "DD_INTERNAL_PAR_USE_DD_URL_FOR_OPMS=true\n", true),
				),
			),
		),
	))
}

func (s *linuxPARSplitSuite) TestSplitControlPlaneEndToEnd() {
	client := s.Env().FakeIntake.Client()
	s.Require().NoError(client.FlushPAR(), "reset PAR state so same-host retries are independent")
	s.resetSigningKeyState()
	s.T().Cleanup(s.resetSigningKeyState)
	s.Require().NoError(client.RCAddConfig("", runnerKeysRCProduct, s.signingKey1.id, s.signingKey1.id, s.signingKey1.config))

	s.waitForProcessStateStable(parControlProcess, "Running", 5*time.Second, 2*time.Minute)
	// Confirm the monolith exits.
	s.waitForSystemdState(privateActionRunnerServiceName, "inactive", 2*time.Minute)
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		count, err := client.GetPARDequeueCount()
		require.NoError(c, err)
		require.Greater(c, count, 0, "par-control should poll OPMS")
	}, 2*time.Minute, 2*time.Second)
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		count, err := client.GetPARHealthCheckCount()
		require.NoError(c, err)
		require.Greater(c, count, 0, "par-control should report runner liveness")
	}, 90*time.Second, 5*time.Second)

	// The executor definition must stay cold until work arrives.
	s.waitForProcessState(parExecutorProcess, "Created", 2*time.Minute)

	setPARTaskSigningKey(s.T(), client, s.signingKey1)
	taskID := uuid.New().String()
	s.Require().NoError(client.EnqueuePARTask(taskID, runCommandAction, map[string]interface{}{
		"command":         "echo par-split-e2e",
		"allowedCommands": []string{"rshell:echo"},
	}))
	s.waitForProcessState(parExecutorProcess, "Running", 2*time.Minute)
	// Deliver the key asynchronously while the cold executor registers its subscription.
	s.deliverSigningKeyAfterSubscription(s.signingKey1)
	result, err := client.GetPARTaskResult(taskID, 2*time.Minute)
	s.Require().NoError(err)
	s.Require().True(result.Success, "split PAR action failed: %+v", result)
	s.Require().Equal(0, rshellExitCode(s.T(), result), "unexpected rshell result: %+v", result)
	s.Require().Contains(result.Outputs["stdout"], "par-split-e2e")

	s.testCoreAgentUnavailableRecovery()
	s.testSigningKeyLifecycle()

	// Successful execution proves the cold executor was started on demand.
	s.waitForProcessState(parExecutorProcess, "Exited", 3*time.Minute)

	// Exercise bootstrap and identity selection through the installed process
	// definition. These transitions share one ordered test because they mutate
	// datadog.yaml and the persisted identity file.
	s.testBootstrapIdentityScenarios()

}

func (s *linuxPARSplitSuite) testCoreAgentUnavailableRecovery() {
	client := s.Env().FakeIntake.Client()
	s.waitForProcessState(parExecutorProcess, "Exited", 3*time.Minute)

	host := s.Env().RemoteHost
	// Stopping the service also stops procmgr through systemd's BindsTo relationship.
	pauseAgent := "sudo systemctl kill --kill-who=main --signal=SIGSTOP " + coreAgentServiceName
	resumeAgent := "sudo systemctl kill --kill-who=main --signal=SIGCONT " + coreAgentServiceName
	_, err := host.Execute(pauseAgent)
	s.Require().NoError(err)
	s.T().Cleanup(func() { _, _ = host.Execute(resumeAgent) })

	setPARTaskSigningKey(s.T(), client, s.signingKey1)
	taskID := uuid.New().String()
	s.Require().NoError(client.EnqueuePARTask(taskID, runCommandAction, map[string]interface{}{
		"command":         "echo core-agent-recovered",
		"allowedCommands": []string{"rshell:echo"},
	}))
	s.waitForProcessState(parExecutorProcess, "Running", 2*time.Minute)
	_, err = client.GetPARTaskResult(taskID, 5*time.Second)
	s.Require().Error(err, "task should remain queued while the Core Agent is unavailable")

	_, err = host.Execute(resumeAgent)
	s.Require().NoError(err)
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		_, statusErr := host.Execute("sudo datadog-agent status")
		require.NoError(c, statusErr)
	}, 2*time.Minute, 5*time.Second, "core Agent should recover")
	// This is a fresh executor; deliver the key as an RC update
	// after its subscription can reach the resumed Core Agent.
	s.deliverSigningKeyAfterSubscription(s.signingKey1)

	result, err := client.GetPARTaskResult(taskID, 2*time.Minute)
	s.Require().NoError(err)
	s.Require().True(result.Success, "task queued during Core Agent outage should execute after recovery: %+v", result)
	s.Require().Contains(result.Outputs["stdout"], "core-agent-recovered")
}

func (s *linuxPARSplitSuite) testSigningKeyLifecycle() {
	client := s.Env().FakeIntake.Client()
	executorPID := strings.TrimSpace(s.Env().RemoteHost.MustExecute(
		`pgrep -fo '[/]opt/datadog-agent/embedded/bin/privateactionrunner run-executor'`,
	))
	s.Require().NotEmpty(executorPID)

	// Delete key 1 before adding key 2 so a successful key-2 task proves that
	// this executor consumed the complete replacement snapshot.
	s.deleteSigningKey(s.signingKey1.id)
	s.Require().NoError(client.RCAddConfig("", runnerKeysRCProduct, s.signingKey2.id, s.signingKey2.id, s.signingKey2.config))
	setPARTaskSigningKey(s.T(), client, s.signingKey2)
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		taskID := uuid.New().String()
		require.NoError(c, client.EnqueuePARTask(taskID, runCommandAction, map[string]interface{}{
			"command":         "echo rotated-key",
			"allowedCommands": []string{"rshell:echo"},
		}))
		result, err := client.GetPARTaskResult(taskID, 20*time.Second)
		require.NoError(c, err)
		require.True(c, result.Success, "rotated signing key should execute: %+v", result)
	}, time.Minute, 2*time.Second)

	revoked := s.runSignedTask(s.signingKey1, "revoked-key")
	s.Require().False(revoked.Success, "revoked signing key should be rejected")
	s.Require().Equal(int(errorcode.ActionPlatformErrorCode_SIGNATURE_KEY_NOT_FOUND), revoked.ErrorCode)
	s.Require().Equal(executorPID, strings.TrimSpace(s.Env().RemoteHost.MustExecute(
		`pgrep -fo '[/]opt/datadog-agent/embedded/bin/privateactionrunner run-executor'`,
	)), "key rotation and revocation must happen in the same executor process")
}

func (s *linuxPARSplitSuite) runSignedTask(key testSigningKey, output string) *api.PARTaskResult {
	client := s.Env().FakeIntake.Client()
	setPARTaskSigningKey(s.T(), client, key)
	taskID := uuid.New().String()
	err := client.EnqueuePARTask(taskID, runCommandAction, map[string]interface{}{
		"command":         "echo " + output,
		"allowedCommands": []string{"rshell:echo"},
	})
	s.Require().NoError(err)
	result, err := s.Env().FakeIntake.Client().GetPARTaskResult(taskID, 2*time.Minute)
	s.Require().NoError(err)
	return result
}

func (s *linuxPARSplitSuite) resetSigningKeyState() {
	client := s.Env().FakeIntake.Client()
	s.Require().NoError(client.RCSetExpiration(time.Now().Add(24 * time.Hour)))
	configs, err := client.RCListConfigs()
	s.Require().NoError(err)
	for _, config := range configs {
		if config.Product == runnerKeysRCProduct {
			key := fmt.Sprintf("%s/%s/%s/%s", config.OrgID, config.Product, config.ConfigID, config.ConfigName)
			s.Require().NoError(client.RCDeleteConfig(key))
		}
	}
}

// deliverSigningKeyAfterSubscription simulates an asynchronous RC update.
// Repeated versions tolerate the absence of a subscription-readiness signal.
func (s *linuxPARSplitSuite) deliverSigningKeyAfterSubscription(key testSigningKey) {
	client := s.Env().FakeIntake.Client()
	for range 5 {
		s.Require().NoError(client.RCAddConfig("", runnerKeysRCProduct, key.id, key.id, key.config))
		time.Sleep(2 * time.Second)
	}
}

func (s *linuxPARSplitSuite) deleteSigningKey(id string) {
	configs, err := s.Env().FakeIntake.Client().RCListConfigs()
	s.Require().NoError(err)
	for _, config := range configs {
		if config.Product == runnerKeysRCProduct && config.ConfigID == id {
			key := fmt.Sprintf("%s/%s/%s/%s", config.OrgID, config.Product, config.ConfigID, config.ConfigName)
			s.Require().NoError(s.Env().FakeIntake.Client().RCDeleteConfig(key))
			return
		}
	}
	s.Fail("signing-key config not found", id)
}

func (s *linuxPARSplitSuite) testBootstrapIdentityScenarios() {
	host := s.Env().RemoteHost

	// Restore the provisioned inline configuration for same-host retries.
	s.T().Cleanup(s.restoreBaseline)

	client := s.Env().FakeIntake.Client()
	selfEnrollConfig := selfEnrollSplitConfig(client.URL())
	_, _ = host.Execute("sudo rm -f " + parIdentityPath)
	s.restartControl(selfEnrollConfig, "Running")
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		count, err := client.GetPAREnrollmentCount()
		require.NoError(c, err)
		require.Equal(c, 1, count, "first identity-less startup should enroll once")
	}, 2*time.Minute, 2*time.Second)
	firstIdentity := strings.TrimSpace(host.MustExecute("sudo sha256sum " + parIdentityPath + " | cut -d' ' -f1"))

	// A subsequent startup adopts the persisted identity without enrollment or
	// file rotation.
	s.restartControl(selfEnrollConfig, "Running")
	secondIdentity := strings.TrimSpace(host.MustExecute("sudo sha256sum " + parIdentityPath + " | cut -d' ' -f1"))
	s.Require().Equal(firstIdentity, secondIdentity, "valid persisted identity should not rotate")
	count, err := client.GetPAREnrollmentCount()
	s.Require().NoError(err)
	s.Require().Equal(1, count, "valid persisted identity should not enroll again")

	// A hostname mismatch makes bootstrap-par-control replace the stale identity.
	host.MustExecute(
		`sudo sed -i 's/"hostname":"[^"]*"/"hostname":"definitely-not-this-host"/' ` + parIdentityPath,
	)
	s.restartControl(selfEnrollConfig, "Running")
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		count, err := client.GetPAREnrollmentCount()
		require.NoError(c, err)
		require.Equal(c, 2, count, "stale hostname should trigger reenrollment")
	}, 2*time.Minute, 2*time.Second)
	reenrolledIdentity := strings.TrimSpace(host.MustExecute("sudo sha256sum " + parIdentityPath + " | cut -d' ' -f1"))
	s.Require().NotEqual(firstIdentity, reenrolledIdentity, "reenrollment should replace the identity")

	// A legacy persisted identity (without hostname) wins over even a malformed
	// inline URN, because Go resolves identity before deriving the config.
	persisted := fmt.Sprintf(`{"private_key":%q,"urn":%q}`, s.persistedKey, s.persistedURN)
	s.Require().NoError(s.writeIdentity(persisted))
	persistedConfig := splitConfig("not-a-runner-urn", s.inlineKey)
	s.restartControl(persistedConfig, "Running")

	// A stale hostname is removed by bootstrap-par-control. The configured inline
	// identity then lets startup continue without contacting enrollment.
	stale := fmt.Sprintf(`{"private_key":%q,"urn":%q,"hostname":"definitely-not-this-host"}`, s.persistedKey, s.persistedURN)
	s.Require().NoError(s.writeIdentity(stale))
	s.restartControl(s.baselineConfig, "Running")
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		_, err := host.Execute("sudo test ! -e " + parIdentityPath)
		require.NoError(c, err)
	}, 10*time.Second, time.Second, "stale persisted identity should be removed")

	// With enrollment disabled, no persisted or inline identity is a startup
	// error rather than an attempted enrollment.
	_, _ = host.Execute("sudo rm -f " + parIdentityPath)
	s.restartControl(splitConfig("", ""), "Failed")

	// Direct Rust settings never resolve secret-backend handles. The process must
	// fail before constructing the OPMS client, and must not persist the handle.
	encConfig := splitConfig(s.inlineURN, "ENC[runner-private-key]")
	s.restartControl(encConfig, "Failed")
	host.MustExecute("sudo test ! -e " + parIdentityPath)

}

func (s *linuxPARSplitSuite) restoreBaseline() {
	host := s.Env().RemoteHost
	_ = s.runProcmgr("stop", parControlProcess)
	s.waitForProcessInactive(parControlProcess, 10*time.Second)
	s.Require().NoError(s.writeConfig(s.baselineConfig))
	_, _ = host.Execute("sudo rm -f " + parIdentityPath)
	s.Require().NoError(s.runProcmgr("start", parControlProcess))
	s.waitForProcessState(parControlProcess, "Running", 2*time.Minute)
}

func selfEnrollSplitConfig(fakeintakeURL string) string {
	return fmt.Sprintf(`dd_url: %q
skip_ssl_validation: true
private_action_runner:
  enabled: true
  split_enabled: true
  self_enroll: true
  idle_timeout_seconds: 5
  actions_allowlist:
    - %s
`, fakeintakeURL, runCommandAction)
}

func splitConfig(urn, privateKey string) string {
	identity := ""
	if urn != "" {
		identity += fmt.Sprintf("  urn: %s\n", urn)
	}
	if privateKey != "" {
		identity += fmt.Sprintf("  private_key: %s\n", privateKey)
	}
	return fmt.Sprintf(`private_action_runner:
  enabled: true
  split_enabled: true
  self_enroll: false
%s  idle_timeout_seconds: 120
  actions_allowlist:
    - %s
`, identity, runCommandAction)
}

func (s *linuxPARSplitSuite) restartControl(config, expectedState string) {
	_ = s.runProcmgr("stop", parControlProcess)
	s.waitForProcessInactive(parControlProcess, 10*time.Second)
	s.Require().NoError(s.writeConfig(config))
	s.Require().NoError(s.runProcmgr("start", parControlProcess))
	s.waitForProcessState(parControlProcess, expectedState, 2*time.Minute)
}

func (s *linuxPARSplitSuite) writeConfig(config string) error {
	host := s.Env().RemoteHost
	if _, err := host.WriteFile(parConfigStaging, []byte(config)); err != nil {
		return err
	}
	_, err := host.Execute(fmt.Sprintf(
		"sudo install -o root -g dd-agent -m 0640 %s %s",
		parConfigStaging,
		privateActionRunnerConfigPath,
	))
	return err
}

func (s *linuxPARSplitSuite) writeIdentity(identity string) error {
	host := s.Env().RemoteHost
	if _, err := host.WriteFile(parConfigStaging, []byte(identity)); err != nil {
		return err
	}
	_, err := host.Execute(fmt.Sprintf(
		"sudo install -o dd-agent -g dd-agent -m 0600 %s %s",
		parConfigStaging,
		parIdentityPath,
	))
	return err
}

func (s *linuxPARSplitSuite) runProcmgr(command, name string) error {
	_, err := s.Env().RemoteHost.Execute(fmt.Sprintf(
		"sudo %s --socket %s %s %s", procmgrCLI, procmgrSocket, command, name,
	))
	return err
}

func (s *linuxPARSplitSuite) waitForProcessState(name, state string, timeout time.Duration) {
	s.T().Helper()
	s.waitForProcessStateStable(name, state, 0, timeout)
}

func (s *linuxPARSplitSuite) waitForProcessInactive(name string, timeout time.Duration) {
	s.T().Helper()
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		output, err := s.Env().RemoteHost.Execute(fmt.Sprintf(
			"sudo %s --socket %s describe %s", procmgrCLI, procmgrSocket, name,
		))
		require.NoError(c, err)
		state := strings.ReplaceAll(output, " ", "")
		require.True(c, strings.Contains(state, "State:Stopped") || strings.Contains(state, "State:Failed"))
	}, timeout, time.Second, "%s should stop running", name)
}

func (s *linuxPARSplitSuite) waitForProcessStateStable(name, state string, stableFor, timeout time.Duration) {
	s.T().Helper()
	var lastOutput string
	var matchingSince time.Time
	ok := assert.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		output, err := s.Env().RemoteHost.Execute(fmt.Sprintf(
			"sudo %s --socket %s describe %s", procmgrCLI, procmgrSocket, name,
		))
		lastOutput = output
		if !assert.NoError(c, err) {
			matchingSince = time.Time{}
			return
		}
		if !assert.Contains(c, strings.ReplaceAll(output, " ", ""), "State:"+state) {
			matchingSince = time.Time{}
			return
		}
		if matchingSince.IsZero() {
			matchingSince = time.Now()
		}
		assert.GreaterOrEqual(c, time.Since(matchingSince), stableFor)
	}, timeout, 2*time.Second, "%s should become %s and remain there for %s", name, state, stableFor)
	if !ok {
		journal, _ := s.Env().RemoteHost.Execute(
			"sudo journalctl -u datadog-agent-procmgr.service --no-pager -n 500 --output=cat",
		)
		s.Require().FailNowf("process did not reach expected state",
			"%s should become %s and remain there for %s\nlast describe output:\n%s\ndd-procmgrd journal tail:\n%s",
			name, state, stableFor, lastOutput, journal)
	}
}

func (s *linuxPARSplitSuite) waitForSystemdState(name, state string, timeout time.Duration) {
	s.T().Helper()
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		output, err := s.Env().RemoteHost.Execute(fmt.Sprintf("sudo systemctl is-active %s || true", name))
		require.NoError(c, err)
		require.Equal(c, state, strings.TrimSpace(output))
	}, timeout, 2*time.Second, "%s should become %s", name, state)
}
