// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunner

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

const (
	rshellBundleFQNPrefix       = "com.datadoghq.remoteaction.rshell"
	runCommandAction            = rshellBundleFQNPrefix + ".runCommand"
	runRemediationCommandAction = rshellBundleFQNPrefix + ".runRemediationCommand"

	parContainerName = "private-action-runner"
	agentNamespace   = "datadog"

	// The provisioner plants both files on the Kind node. The operator policy
	// admits only the allowed subtree under the PAR container's /host mount.
	testDataFile           = "/host/var/log/par-e2e-allowed/testdata.txt"
	testDataContent        = "PAR_E2E_VALUE=hello_from_rshell"
	operatorBlockedFile    = "/host/var/log/par-e2e-blocked/testdata.txt"
	operatorBlockedContent = "PAR_E2E_BLOCKED_VALUE=operator_path_must_block"
)

type parK8sSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
	runnerURN string
}

func TestPARRshellK8sSuite(t *testing.T) {
	t.Parallel()
	urn, keyB64 := generateTestRunnerIdentity(t)
	suite := &parK8sSuite{runnerURN: urn}
	e2e.Run(t, suite, e2e.WithProvisioner(parK8sProvisioner(urn, keyB64)))
}

// SetupSuite waits for PAR to be ready and actively polling fakeintake.
func (s *parK8sSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	defer s.CleanupOnSetupFailure()
	s.waitForPARReady()
}

func (s *parK8sSuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)
	if !s.IsDevMode() {
		_ = s.Env().FakeIntake.Client().FlushPAR()
	}
}

// TestRshellHappyFlow verifies the deployed operator and backend policies overlap.
// The backend path is broader than the operator's allowed test-data subtree.
func (s *parK8sSuite) TestRshellHappyFlow() {
	taskID := uuid.New().String()
	err := s.Env().FakeIntake.Client().EnqueuePARTask(taskID, runCommandAction, map[string]interface{}{
		"command":         "cat " + testDataFile,
		"allowedCommands": []string{"rshell:cat"},
		"allowedPaths":    []string{"/host/var/log"},
	})
	s.Require().NoError(err)

	result := s.pollResult(taskID, 2*time.Minute)
	s.Require().Equal(taskID, result.TaskID)
	s.Require().True(result.Success, "unexpected PAR rshell result: %+v", result)
	s.Require().Zero(result.ErrorCode)
	s.Require().Empty(result.ErrorDetails)
	s.Require().Equal(0, rshellExitCode(s.T(), result), "unexpected PAR rshell result: %+v", result)
	assert.Contains(s.T(), result.Outputs["stdout"], testDataContent)
	assert.Equal(s.T(), "", result.Outputs["stderr"])
	assert.NotContains(s.T(), result.Outputs, "sandboxWarnings")
}

// TestRshellOperatorCommandPolicyNarrowsBackendPolicy verifies the backend can
// admit a command that the deployed operator policy still rejects.
func (s *parK8sSuite) TestRshellOperatorCommandPolicyNarrowsBackendPolicy() {
	taskID := uuid.New().String()
	err := s.Env().FakeIntake.Client().EnqueuePARTask(taskID, runCommandAction, map[string]interface{}{
		"command":         "ls " + testDataFile,
		"allowedCommands": []string{"rshell:ls"},
		"allowedPaths":    []string{"/host/var/log"},
	})
	s.Require().NoError(err)

	result := s.pollResult(taskID, 2*time.Minute)
	s.Require().True(result.Success, "rshell policy rejection should be a completed PAR task")
	assert.Equal(s.T(), 127, rshellExitCode(s.T(), result))
	assert.Contains(s.T(), result.Outputs["stderr"], "command not allowed")
	assert.NotContains(s.T(), result.Outputs["stdout"], testDataContent)
}

// TestRshellOperatorPathPolicyNarrowsBackendPolicy verifies the backend can
// admit a path that the deployed operator policy still rejects.
func (s *parK8sSuite) TestRshellOperatorPathPolicyNarrowsBackendPolicy() {
	taskID := uuid.New().String()
	err := s.Env().FakeIntake.Client().EnqueuePARTask(taskID, runCommandAction, map[string]interface{}{
		"command":         "cat " + operatorBlockedFile,
		"allowedCommands": []string{"rshell:cat"},
		"allowedPaths":    []string{"/host/var/log"},
	})
	s.Require().NoError(err)

	result := s.pollResult(taskID, 2*time.Minute)
	s.Require().True(result.Success, "rshell policy rejection should be a completed PAR task")
	assert.Equal(s.T(), 1, rshellExitCode(s.T(), result))
	assert.Contains(s.T(), result.Outputs["stderr"], "permission denied")
	assert.NotContains(s.T(), result.Outputs["stdout"], operatorBlockedContent)
}

// TestRshellBlockedPath verifies rshell blocks access to paths outside restricted_shell.allowed_paths.
func (s *parK8sSuite) TestRshellBlockedPath() {
	taskID := uuid.New().String()
	err := s.Env().FakeIntake.Client().EnqueuePARTask(taskID, runCommandAction, map[string]interface{}{
		"command":         "cat /etc/passwd",
		"allowedCommands": []string{"rshell:cat"},
	})
	s.Require().NoError(err)

	result := s.pollResult(taskID, 2*time.Minute)
	assert.NotEqual(s.T(), 0, rshellExitCode(s.T(), result), "expected non-zero exit code for blocked path")
	assert.NotEmpty(s.T(), result.Outputs["stderr"], "expected error message in stderr")
}

// TestRshellBlockedCommand verifies rshell blocks commands not in allowedCommands.
func (s *parK8sSuite) TestRshellBlockedCommand() {
	taskID := uuid.New().String()
	err := s.Env().FakeIntake.Client().EnqueuePARTask(taskID, runCommandAction, map[string]interface{}{
		"command":         "grep PAR_E2E_VALUE " + testDataFile,
		"allowedCommands": []string{"rshell:echo", "rshell:cat"},
		"allowedPaths":    []string{"/host/var/log"},
	})
	s.Require().NoError(err)

	result := s.pollResult(taskID, 2*time.Minute)
	s.Require().True(result.Success, "rshell policy rejection should be a completed PAR task")
	s.Require().Zero(result.ErrorCode)
	s.Require().Empty(result.ErrorDetails)
	assert.Equal(s.T(), 127, rshellExitCode(s.T(), result))
	assert.Contains(s.T(), result.Outputs["stderr"], "command not allowed")
}

// TestRshellBlockedExecCmd verifies rshell blocks the command inside -exec when it is not
// in allowedCommands. find itself is allowed but rm is not, so the -exec validation fails.
func (s *parK8sSuite) TestRshellBlockedExecCmd() {
	taskID := uuid.New().String()
	err := s.Env().FakeIntake.Client().EnqueuePARTask(taskID, runCommandAction, map[string]interface{}{
		"command":         fmt.Sprintf("find %s -exec rm {} \\;", testDataFile),
		"allowedCommands": []string{"rshell:find"},
		"allowedPaths":    []string{"/host/var/log"},
	})
	s.Require().NoError(err)

	result := s.pollResult(taskID, 2*time.Minute)
	assert.Equal(s.T(), 1, rshellExitCode(s.T(), result))
	assert.Contains(s.T(), result.Outputs["stderr"], "command not allowed")
}

// TestRshellSandboxWarningsPublishedSeparatelyFromStderr verifies sandbox
// diagnostics survive the PAR result wire format without becoming command stderr.
func (s *parK8sSuite) TestRshellSandboxWarningsPublishedSeparatelyFromStderr() {
	missingPath := "/host/var/log/par-e2e-allowed/missing-" + uuid.New().String()
	taskID := uuid.New().String()
	err := s.Env().FakeIntake.Client().EnqueuePARTask(taskID, runCommandAction, map[string]interface{}{
		"command":         "echo hello",
		"allowedCommands": []string{"rshell:echo"},
		"allowedPaths":    []string{missingPath},
	})
	s.Require().NoError(err)

	result := s.pollResult(taskID, 2*time.Minute)
	s.Require().Equal(taskID, result.TaskID)
	s.Require().True(result.Success, "sandbox warnings should not fail the PAR task")
	s.Require().Zero(result.ErrorCode)
	s.Require().Empty(result.ErrorDetails)
	s.Require().Equal(0, rshellExitCode(s.T(), result))
	assert.Equal(s.T(), "hello\n", result.Outputs["stdout"])
	assert.Equal(s.T(), "", result.Outputs["stderr"])

	rawWarnings, ok := result.Outputs["sandboxWarnings"]
	s.Require().True(ok, "expected sandboxWarnings in result outputs")
	warnings, ok := rawWarnings.([]interface{})
	s.Require().True(ok, "unexpected sandboxWarnings type %T", rawWarnings)
	s.Require().Len(warnings, 1)
	warning, ok := warnings[0].(string)
	s.Require().True(ok, "unexpected sandbox warning type %T", warnings[0])
	assert.Contains(s.T(), warning, "AllowedPaths: skipping")
	assert.Contains(s.T(), warning, missingPath)
}

// TestRshellRemediationWriteFile verifies that the runRemediationCommand action runs
// rshell in remediation mode, allowing a file-target output redirection inside an
// allowed path. runCommand (read-only mode) would reject the same redirection. The
// command writes a file and reads it back to confirm the write landed on disk.
//
// The target lives under /tmp (the PAR container's own writable filesystem), not the
// /host/* mounts used by the read tests: host paths are mounted read-only, so a write
// there would fail regardless of rshell's mode.
func (s *parK8sSuite) TestRshellRemediationWriteFile() {
	target := "/tmp/par-e2e-remediation.txt"
	content := "PAR_REMEDIATION_VALUE=written_by_rshell"

	taskID := uuid.New().String()
	err := s.Env().FakeIntake.Client().EnqueuePARTask(taskID, runRemediationCommandAction, map[string]interface{}{
		"command":         fmt.Sprintf("echo %s > %s && cat %s", content, target, target),
		"allowedCommands": []string{"rshell:echo", "rshell:cat"},
		"allowedPaths":    []string{"/tmp:rw"},
	})
	s.Require().NoError(err)

	result := s.pollResult(taskID, 2*time.Minute)
	s.Require().Equal(0, rshellExitCode(s.T(), result), "unexpected PAR rshell result: %+v", result)
	assert.Contains(s.T(), result.Outputs["stdout"], content)
}

// TestRshellRemediationReadOnlyPathBlocksWrite verifies remediation mode does
// not upgrade a signed read-only path to read-write access.
func (s *parK8sSuite) TestRshellRemediationReadOnlyPathBlocksWrite() {
	target := "/var/tmp/par-e2e-remediation-ro-" + uuid.New().String() + ".txt"
	taskID := uuid.New().String()
	err := s.Env().FakeIntake.Client().EnqueuePARTask(taskID, runRemediationCommandAction, map[string]interface{}{
		"command":         "echo should_not_write > " + target,
		"allowedCommands": []string{"rshell:echo"},
		"allowedPaths":    []string{"/var/tmp:ro"},
	})
	s.Require().NoError(err)

	result := s.pollResult(taskID, 2*time.Minute)
	s.Require().True(result.Success, "rshell path rejection should be a completed PAR task")
	assert.Equal(s.T(), 1, rshellExitCode(s.T(), result))
	assert.Equal(s.T(), "", result.Outputs["stdout"])
	assert.Contains(s.T(), result.Outputs["stderr"], "permission denied")
	assert.NotContains(s.T(), result.Outputs, "sandboxWarnings")

	verifyTaskID := uuid.New().String()
	err = s.Env().FakeIntake.Client().EnqueuePARTask(verifyTaskID, runCommandAction, map[string]interface{}{
		"command":         "cat " + target,
		"allowedCommands": []string{"rshell:cat"},
		"allowedPaths":    []string{"/var/tmp:ro"},
	})
	s.Require().NoError(err)

	verifyResult := s.pollResult(verifyTaskID, 2*time.Minute)
	assert.Equal(s.T(), 1, rshellExitCode(s.T(), verifyResult))
	assert.Equal(s.T(), "", verifyResult.Outputs["stdout"])
	assert.Contains(s.T(), verifyResult.Outputs["stderr"], "no such file or directory")
	assert.NotContains(s.T(), verifyResult.Outputs, "sandboxWarnings")
}

// TestRshellRunCommandBlocksWrite verifies that the read-only runCommand action rejects
// a file-target output redirection even when the path is allowed — the security boundary
// that distinguishes it from runRemediationCommand.
func (s *parK8sSuite) TestRshellRunCommandBlocksWrite() {
	taskID := uuid.New().String()
	err := s.Env().FakeIntake.Client().EnqueuePARTask(taskID, runCommandAction, map[string]interface{}{
		"command":         "echo nope > /tmp/par-e2e-readonly.txt",
		"allowedCommands": []string{"rshell:echo"},
		"allowedPaths":    []string{"/tmp:rw"},
	})
	s.Require().NoError(err)

	result := s.pollResult(taskID, 2*time.Minute)
	assert.NotEqual(s.T(), 0, rshellExitCode(s.T(), result), "read-only runCommand must reject file-target redirections")
}

// --- helpers ---

func (s *parK8sSuite) pollResult(taskID string, timeout time.Duration) *api.PARTaskResult {
	result, err := s.Env().FakeIntake.Client().GetPARTaskResult(taskID, timeout)
	s.Require().NoError(err, "timed out waiting for task result")
	return result
}

// rshellExitCode requires and extracts the integer exit code from a task result.
// rshell reports all outcomes (including blocked commands) as successful PAR tasks
// with a non-zero exit code, so tests check exitCode rather than result.Success.
func rshellExitCode(t *testing.T, result *api.PARTaskResult) int {
	t.Helper()
	require.NotNil(t, result, "result is nil")
	require.NotNil(t, result.Outputs, "result has no outputs: %+v", result)
	value, ok := result.Outputs["exitCode"]
	require.True(t, ok, "result has no exitCode output: %+v", result)
	exitCode, ok := value.(float64) // JSON numbers decode as float64
	require.True(t, ok, "unexpected exitCode type %T", value)
	return int(exitCode)
}

// waitForPARReady waits until the private-action-runner container is Ready
// and fakeintake is reachable (confirming the ECS task is up).
func (s *parK8sSuite) waitForPARReady() {
	selector := s.Env().Agent.LinuxNodeAgent.LabelSelectors["app"]
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		pods, err := s.Env().KubernetesCluster.Client().CoreV1().
			Pods(agentNamespace).List(context.Background(), metav1.ListOptions{
			LabelSelector: "app=" + selector,
		})
		assert.NoError(c, err)
		for _, pod := range pods.Items {
			for _, cs := range pod.Status.ContainerStatuses {
				if cs.Name == parContainerName && cs.Ready {
					return
				}
			}
		}
		assert.Fail(c, "private-action-runner container not ready")
	}, 5*time.Minute, 10*time.Second, "PAR container should become ready")

	// Confirm PAR is actively polling fakeintake by waiting for at least one dequeue call.
	// This guards against a race where the container is Ready but the dequeue loop hasn't started.
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		count, err := s.Env().FakeIntake.Client().GetPARDequeueCount()
		assert.NoError(c, err)
		assert.Greater(c, count, 0, "PAR has not yet called the dequeue endpoint")
	}, 2*time.Minute, 3*time.Second, "PAR should start polling fakeintake")
}
