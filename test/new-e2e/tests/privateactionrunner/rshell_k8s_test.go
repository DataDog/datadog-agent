// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunner

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"

	fakeopsClient "github.com/DataDog/datadog-agent/test/fakeopms/client"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
)

const (
	rshellBundleFQNPrefix = "com.datadoghq.remoteaction.rshell"
	runCommandAction      = rshellBundleFQNPrefix + ".runCommand"

	parContainerName = "private-action-runner"
	agentNamespace   = "datadog"

	// testDataFile is planted on the Kind node during provisioning.
	testDataFile    = "/tmp/par-e2e-testdata.txt"
	testDataContent = "PAR_E2E_VALUE=hello_from_rshell"
)

type parK8sSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
	fakeOpms  *fakeopsClient.Client
	runnerURN string
	stopPF    chan struct{} // closes the port-forward goroutine
}

func TestPARRshellK8sSuite(t *testing.T) {
	t.Parallel()
	urn, keyB64 := generateTestRunnerIdentity(t)
	suite := &parK8sSuite{runnerURN: urn}
	e2e.Run(t, suite, e2e.WithProvisioner(parK8sProvisioner(urn, keyB64)))
}

// SetupSuite provisions infra, plants the test data file, and waits for PAR to be ready.
func (s *parK8sSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	defer s.CleanupOnSetupFailure()

	// Plant test data file on all agent pods' /tmp via PodExec.
	s.plantTestDataFile()

	// Port-forward fake OPMS service to a local port so the test process can call it.
	localPort, stopCh, err := s.startPortForward(fakeOpmsNamespace, fakeOpmsName, fakeOpmsPort)
	s.Require().NoError(err, "failed to port-forward fake OPMS service")
	s.stopPF = stopCh
	s.fakeOpms = fakeopsClient.NewClient(fmt.Sprintf("http://localhost:%d", localPort))

	// Wait for PAR container to be ready and actively polling the fake OPMS.
	s.waitForPARReady()
}

func (s *parK8sSuite) TearDownSuite() {
	if s.stopPF != nil {
		close(s.stopPF)
	}
	s.BaseSuite.TearDownSuite()
}

func (s *parK8sSuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)
	if !s.IsDevMode() {
		_ = s.fakeOpms.Flush()
	}
}

// TestRshellHappyFlow verifies PAR can execute a complex rshell script:
// find the planted test file, read it, grep for the expected value.
func (s *parK8sSuite) TestRshellHappyFlow() {
	taskID := uuid.New().String()
	err := s.fakeOpms.Enqueue(taskID, runCommandAction, map[string]interface{}{
		"command": fmt.Sprintf("find /tmp -name par-e2e-testdata.txt | xargs cat | grep %q", "PAR_E2E_VALUE"),
	})
	s.Require().NoError(err)

	result := s.pollResult(taskID, 2*time.Minute)
	s.Require().True(result.Success, "expected success, got error: %s", result.ErrorDetails)
	assert.Contains(s.T(), result.Outputs["stdout"], testDataContent)
}

// TestRshellBlockedPath verifies PAR rejects access to paths outside restricted_shell_allowed_paths.
func (s *parK8sSuite) TestRshellBlockedPath() {
	taskID := uuid.New().String()
	err := s.fakeOpms.Enqueue(taskID, runCommandAction, map[string]interface{}{
		"command": "cat /etc/passwd",
	})
	s.Require().NoError(err)

	result := s.pollResult(taskID, 2*time.Minute)
	s.Require().False(result.Success, "expected failure for blocked path")
	assert.NotEmpty(s.T(), result.ErrorDetails, "expected a meaningful error message")
}

// TestRshellBlockedCommand verifies PAR rejects commands not in allowedCommands.
func (s *parK8sSuite) TestRshellBlockedCommand() {
	taskID := uuid.New().String()
	err := s.fakeOpms.Enqueue(taskID, runCommandAction, map[string]interface{}{
		"command":         fmt.Sprintf("grep PAR_E2E_VALUE %s", testDataFile),
		"allowedCommands": []string{"echo", "cat"},
	})
	s.Require().NoError(err)

	result := s.pollResult(taskID, 2*time.Minute)
	s.Require().False(result.Success, "expected failure for blocked command")
	assert.NotEmpty(s.T(), result.ErrorDetails, "expected a meaningful error message")
}

// TestRshellBlockedFlag verifies PAR rejects unsupported flags (-exec on find is not supported by rshell).
func (s *parK8sSuite) TestRshellBlockedFlag() {
	taskID := uuid.New().String()
	err := s.fakeOpms.Enqueue(taskID, runCommandAction, map[string]interface{}{
		"command": "find /tmp -name par-e2e-testdata.txt -exec cat {} \\;",
	})
	s.Require().NoError(err)

	result := s.pollResult(taskID, 2*time.Minute)
	s.Require().False(result.Success, "expected failure for unsupported -exec flag")
	assert.NotEmpty(s.T(), result.ErrorDetails, "expected a meaningful error message")
}

// --- helpers ---

func (s *parK8sSuite) pollResult(taskID string, timeout time.Duration) *fakeopsClient.TaskResult {
	result, err := s.fakeOpms.PollResult(taskID, timeout)
	s.Require().NoError(err, "timed out waiting for task result")
	return result
}

// waitForPARReady waits until the private-action-runner container is Ready
// and the fake OPMS has received at least one health-check call from PAR.
func (s *parK8sSuite) waitForPARReady() {
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		pods, err := s.Env().KubernetesCluster.Client().CoreV1().
			Pods(agentNamespace).List(context.Background(), metav1.ListOptions{
			LabelSelector: "app=datadog-agent",
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

	// Ensure PAR is actively polling by checking fake OPMS health-check hit count.
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		err := s.fakeOpms.GetHealth()
		assert.NoError(c, err)
	}, 30*time.Second, 2*time.Second, "fake OPMS should be reachable")
}

// plantTestDataFile writes the test data file onto the Kind node so rshell scripts can access it.
func (s *parK8sSuite) plantTestDataFile() {
	pods, err := s.Env().KubernetesCluster.Client().CoreV1().
		Pods(agentNamespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: "app=datadog-agent",
	})
	s.Require().NoError(err)
	s.Require().NotEmpty(pods.Items, "no agent pods found")

	for _, pod := range pods.Items {
		_, _, err = s.Env().KubernetesCluster.KubernetesClient.PodExec(
			agentNamespace, pod.Name, parContainerName,
			[]string{"sh", "-c", fmt.Sprintf("echo %q > %s", testDataContent, testDataFile)},
		)
		s.Require().NoError(err, "failed to plant test data file in pod %s", pod.Name)
	}
}

// startPortForward creates a Kubernetes port-forward from a local port to the fake OPMS service.
// Returns the local port and a stop channel.
func (s *parK8sSuite) startPortForward(namespace, serviceName string, remotePort int) (int, chan struct{}, error) {
	k8sClient := s.Env().KubernetesCluster.KubernetesClient

	// Find a pod backing the service.
	pods, err := k8sClient.K8sClient.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app=%s", serviceName),
	})
	if err != nil || len(pods.Items) == 0 {
		return 0, nil, fmt.Errorf("no pods found for service %s: %w", serviceName, err)
	}
	podName := pods.Items[0].Name

	// Build the SPDY upgrade URL.
	restClient := k8sClient.K8sClient.CoreV1().RESTClient()
	req := restClient.Post().
		Resource("pods").
		Namespace(namespace).
		Name(podName).
		SubResource("portforward")

	transport, upgrader, err := spdy.RoundTripperFor(k8sClient.K8sConfig)
	if err != nil {
		return 0, nil, fmt.Errorf("spdy.RoundTripperFor: %w", err)
	}
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, req.URL())

	stopCh := make(chan struct{})
	readyCh := make(chan struct{})
	// Use port 0 to get an OS-assigned free port.
	ports := []string{fmt.Sprintf("0:%d", remotePort)}
	fw, err := portforward.New(dialer, ports, stopCh, readyCh, nil, nil)
	if err != nil {
		return 0, nil, fmt.Errorf("portforward.New: %w", err)
	}

	go func() { _ = fw.ForwardPorts() }()
	<-readyCh

	forwardedPorts, err := fw.GetPorts()
	if err != nil || len(forwardedPorts) == 0 {
		close(stopCh)
		return 0, nil, fmt.Errorf("could not get forwarded ports: %w", err)
	}

	return int(forwardedPorts[0].Local), stopCh, nil
}
