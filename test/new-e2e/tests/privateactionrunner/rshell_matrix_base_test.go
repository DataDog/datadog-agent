// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Helpers shared between the rshell matrix suites. The permission model has two
// allow-lists (commands and paths) with two sources (backend per-task, operator
// via datadog.yaml) intersected on each task execution. Each matrix suite fixes
// the operator config at provision time and varies the backend lists per test.
//
// Truth table (Confluence: agent / Rshell permission model):
//
//	| Backend ↓ \ Operator → | unset | []     | disjoint | overlapping |
//	| nil / []               | ∅     | ∅      | ∅        | ∅           |
//	| non-empty              | backend | ∅    | ∅        | intersection|

package privateactionrunner

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

// permissiveBackendPath is a path the provisioner plants on the Kind node and that
// rshell's backend allowedPaths list should contain whenever the test wants the
// paths axis to *pass* (so only the commands axis gates the outcome).
const permissiveBackendPath = "/host/var/log"

// permissiveBackendCommand is a command the test can run when it wants the commands
// axis to *pass* (so only the paths axis gates the outcome). The backend's
// allowedCommands list must include it for commands-axis to be permissive.
const permissiveBackendCommand = "rshell:cat"

// matrixSuite is the common base all rshell-matrix suites embed. It owns the
// runner identity and the PAR-readiness probe. Per-suite overrides (SetupSuite,
// BeforeTest) are not needed — the defaults suffice.
type matrixSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
	runnerURN string
}

// SetupSuite waits for PAR to come up and start polling fakeintake before tests run.
func (s *matrixSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	defer s.CleanupOnSetupFailure()
	waitForPARReady(s.T(), s.Env(), s.Require())
}

// BeforeTest flushes fakeintake so each test starts from a clean queue/result state.
func (s *matrixSuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)
	if !s.IsDevMode() {
		_ = s.Env().FakeIntake.Client().FlushPAR()
	}
}

// pollResult is a thin wrapper around GetPARTaskResult with the standard timeout
// and a failed-require on timeout. Returned result is never nil on success.
func (s *matrixSuite) pollResult(taskID string) *api.PARTaskResult {
	result, err := s.Env().FakeIntake.Client().GetPARTaskResult(taskID, 2*time.Minute)
	s.Require().NoError(err, "timed out waiting for task result (taskID=%s)", taskID)
	return result
}

// enqueueAndWait enqueues a runCommand task with the given inputs and blocks
// until fakeintake receives the result. Shared by all matrix suites.
func (s *matrixSuite) enqueueAndWait(inputs map[string]any) *api.PARTaskResult {
	taskID := uuid.New().String()
	s.Require().NoError(s.Env().FakeIntake.Client().EnqueuePARTask(taskID, runCommandAction, inputs))
	return s.pollResult(taskID)
}

// requireInterface is a minimal stand-in for *require.Assertions so waitForPARReady
// can be called from both the suite method and anywhere else without importing the
// suite type. testify's Require() satisfies this.
type requireInterface interface {
	EventuallyWithT(condition func(*assert.CollectT), timeout, interval time.Duration, msgAndArgs ...any)
}

// waitForPARReady waits until the PAR sidecar is Ready and has polled fakeintake
// at least once. Shared across matrix suites — each has its own provisioner but
// the same readiness criteria.
func waitForPARReady(t *testing.T, env *environments.Kubernetes, req requireInterface) {
	t.Helper()

	selector := env.Agent.LinuxNodeAgent.LabelSelectors["app"]
	req.EventuallyWithT(func(c *assert.CollectT) {
		pods, err := env.KubernetesCluster.Client().CoreV1().
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
	// Guards against a race where the container is Ready but the dequeue loop hasn't started.
	req.EventuallyWithT(func(c *assert.CollectT) {
		count, err := env.FakeIntake.Client().GetPARDequeueCount()
		assert.NoError(c, err)
		assert.Greater(c, count, 0, "PAR has not yet called the dequeue endpoint")
	}, 2*time.Minute, 3*time.Second, "PAR should start polling fakeintake")
}

// assertBlocked asserts that a task result indicates rshell blocked the command.
// rshell publishes blocked commands as successful PAR tasks with a non-zero exit
// code, so we check exitCode rather than result.Success.
func assertBlocked(t *testing.T, result *api.PARTaskResult, msgAndArgs ...any) {
	t.Helper()
	assert.NotEqual(t, 0, rshellExitCode(result), msgAndArgs...)
}

// assertAllowed asserts that a task executed successfully end-to-end.
func assertAllowed(t *testing.T, result *api.PARTaskResult, msgAndArgs ...any) {
	t.Helper()
	assert.Equal(t, 0, rshellExitCode(result), msgAndArgs...)
}
