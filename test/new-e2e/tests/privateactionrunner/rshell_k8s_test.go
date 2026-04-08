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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

const (
	rshellBundleFQNPrefix = "com.datadoghq.remoteaction.rshell"
	runCommandAction      = rshellBundleFQNPrefix + ".runCommand"

	parContainerName = "private-action-runner"
	agentNamespace   = "datadog"

	// testDataFile is planted on the Kind node by the provisioner at /var/log/,
	// accessible inside the PAR container at /host/var/log/ via the host volume mount.
	testDataFile    = "/host/var/log/par-e2e-testdata.txt"
	testDataContent = "PAR_E2E_VALUE=hello_from_rshell"
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

// TestRshellHappyFlow verifies PAR can execute a simple rshell command that reads the
// planted test file. allowedCommands must include "rshell:cat" because rshell blocks
// all commands unless explicitly listed.
func (s *parK8sSuite) TestRshellHappyFlow() {
	taskID := uuid.New().String()
	err := s.Env().FakeIntake.Client().EnqueuePARTask(taskID, runCommandAction, map[string]interface{}{
		"command":         "cat " + testDataFile,
		"allowedCommands": []string{"rshell:cat"},
	})
	s.Require().NoError(err)

	result := s.pollResult(taskID, 2*time.Minute)
	s.Require().Equal(0, rshellExitCode(result), "expected exit code 0, got %d (stderr: %v)", rshellExitCode(result), result.Outputs["stderr"])
	assert.Contains(s.T(), result.Outputs["stdout"], testDataContent)
}

// TestRshellBlockedPath verifies rshell blocks access to paths outside restricted_shell_allowed_paths.
func (s *parK8sSuite) TestRshellBlockedPath() {
	taskID := uuid.New().String()
	err := s.Env().FakeIntake.Client().EnqueuePARTask(taskID, runCommandAction, map[string]interface{}{
		"command":         "cat /etc/passwd",
		"allowedCommands": []string{"rshell:cat"},
	})
	s.Require().NoError(err)

	result := s.pollResult(taskID, 2*time.Minute)
	assert.NotEqual(s.T(), 0, rshellExitCode(result), "expected non-zero exit code for blocked path")
	assert.NotEmpty(s.T(), result.Outputs["stderr"], "expected error message in stderr")
}

// TestRshellBlockedCommand verifies rshell blocks commands not in allowedCommands.
func (s *parK8sSuite) TestRshellBlockedCommand() {
	taskID := uuid.New().String()
	err := s.Env().FakeIntake.Client().EnqueuePARTask(taskID, runCommandAction, map[string]interface{}{
		"command":         "grep PAR_E2E_VALUE " + testDataFile,
		"allowedCommands": []string{"rshell:echo", "rshell:cat"},
	})
	s.Require().NoError(err)

	result := s.pollResult(taskID, 2*time.Minute)
	assert.NotEqual(s.T(), 0, rshellExitCode(result), "expected non-zero exit code for blocked command")
	assert.NotEmpty(s.T(), result.Outputs["stderr"], "expected error message in stderr")
}

// TestRshellBlockedExecCmd verifies rshell blocks the command inside -exec when it is not
// in allowedCommands. find itself is allowed but rm is not, so the -exec validation fails.
func (s *parK8sSuite) TestRshellBlockedExecCmd() {
	taskID := uuid.New().String()
	err := s.Env().FakeIntake.Client().EnqueuePARTask(taskID, runCommandAction, map[string]interface{}{
		"command":         fmt.Sprintf("find %s -exec rm {} \\;", testDataFile),
		"allowedCommands": []string{"rshell:find"},
	})
	s.Require().NoError(err)

	result := s.pollResult(taskID, 2*time.Minute)
	assert.NotEqual(s.T(), 0, rshellExitCode(result), "expected non-zero exit code: rm not in allowedCommands so -exec should be blocked")
}

// --- helpers ---

func (s *parK8sSuite) pollResult(taskID string, timeout time.Duration) *api.PARTaskResult {
	result, err := s.Env().FakeIntake.Client().GetPARTaskResult(taskID, timeout)
	s.Require().NoError(err, "timed out waiting for task result")
	return result
}

// rshellExitCode extracts the integer exit code from a task result's outputs.
// rshell reports all outcomes (including blocked commands) as successful PAR tasks
// with a non-zero exit code, so tests check exitCode rather than result.Success.
func rshellExitCode(result *api.PARTaskResult) int {
	if result.Outputs == nil {
		return -1
	}
	v, ok := result.Outputs["exitCode"]
	if !ok {
		return -1
	}
	f, ok := v.(float64) // JSON numbers decode as float64
	if !ok {
		return -1
	}
	return int(f)
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
