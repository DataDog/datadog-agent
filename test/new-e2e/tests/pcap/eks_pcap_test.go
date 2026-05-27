// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package pcap

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	npmtools "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/npm-tools"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	kubeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	awsFakeintake "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/eks"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

// ekspcapEnv embeds the standard Kubernetes environment.  No extra components
// are needed beyond what environments.Kubernetes already provides (cluster,
// agent, fakeintake).
type ekspcapEnv struct {
	environments.Kubernetes
}

// ekspcapSuite is the test suite for Remote PCAP on EKS.
type ekspcapSuite struct {
	e2e.BaseSuite[ekspcapEnv]
}

// ekspcapEnvProvisioner returns the Pulumi provisioner for the PCAP EKS environment.
// It deploys:
//   - An EKS cluster with a Linux node group.
//   - FakeIntake as an ECS Fargate task (serves OPMS endpoints polled by PAR).
//   - Datadog Agent (Helm) with system-probe + PAR enabled, PCAP action allowlisted.
//   - npm-tools workload that continuously generates TCP/DNS traffic for the capture
//     to observe.
func ekspcapEnvProvisioner(opts ...eks.RunOption) provisioners.PulumiEnvRunFunc[ekspcapEnv] {
	return func(ctx *pulumi.Context, env *ekspcapEnv) error {
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return fmt.Errorf("aws.NewEnvironment: %w", err)
		}

		// npm-tools workload — generates curl + dig loops so that there is live
		// traffic to capture once the PCAP action fires.
		npmToolsWorkload := func(_ config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error) {
			return npmtools.K8sAppDefinition(&awsEnv, kubeProvider, "npmtools", "http://google.com/")
		}

		provisionerOpts := []eks.RunOption{
			// Linux x86 node group — system-probe requires Linux kernel eBPF support.
			eks.WithEKSOptions(eks.WithLinuxNodeGroup()),
			// Agent Helm values: NPM + system-probe + PAR with PCAP allowlisted.
			eks.WithAgentOptions(
				kubernetesagentparams.WithHelmValues(pcapHelmValues),
				kubernetesagentparams.WithHelmChartVersion(minHelmChartVersion),
			),
			// FakeIntake: slightly more memory to handle PAR polling alongside
			// NPM payload storage.
			eks.WithFakeIntakeOptions(awsFakeintake.WithMemory(4096)),
			// npm-tools synthetic traffic generator.
			eks.WithWorkloadApp(npmToolsWorkload),
		}
		provisionerOpts = append(provisionerOpts, opts...)

		params := eks.GetRunParams(provisionerOpts...)
		eks.RunWithEnv(ctx, awsEnv, &env.Kubernetes, params)

		return nil
	}
}

// TestEKSPCAPSuite is the entry point for the Remote PCAP EKS test suite.
func TestEKSPCAPSuite(t *testing.T) {
	t.Parallel()

	s := &ekspcapSuite{}
	e2eParams := []e2e.SuiteOption{
		e2e.WithProvisioner(
			provisioners.NewTypedPulumiProvisioner("eks-pcap", ekspcapEnvProvisioner(), nil),
		),
	}
	e2e.Run(t, s, e2eParams...)
}

// SetupSuite waits for PAR to be running and actively polling fakeintake before
// any test runs.  This prevents spurious failures from race conditions at startup.
func (s *ekspcapSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	defer s.CleanupOnSetupFailure()
	s.waitForPARReady()
}

// BeforeTest flushes PAR state between tests so stale tasks/results from a
// previous run do not interfere.
func (s *ekspcapSuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)
	if !s.BaseSuite.IsDevMode() {
		_ = s.Env().FakeIntake.Client().FlushPAR()
	}
}

// Test00PARIsPolling verifies that the Private Action Runner container is up
// and actively calling the dequeue endpoint on fakeintake.  Prefixed "00" so
// it runs first and acts as a health gate: if PAR is not polling, all
// subsequent PCAP tests will time out.
func (s *ekspcapSuite) Test00PARIsPolling() {
	count, err := s.Env().FakeIntake.Client().GetPARDequeueCount()
	s.Require().NoError(err, "failed to query PAR dequeue count")
	s.Require().Greater(count, 0, "PAR has not called the dequeue endpoint — runner may not be started")
}

// TestPCAPRunCaptureHappyFlow enqueues a PCAP runCapture task and verifies that
// PAR executes it and publishes a result containing capture metadata (packet
// count, byte count, or a pcap file reference).
func (s *ekspcapSuite) TestPCAPRunCaptureHappyFlow() {
	taskID := uuid.New().String()

	err := s.Env().FakeIntake.Client().EnqueuePARTask(taskID, pcapActionFQN, map[string]interface{}{
		"interface":       defaultCaptureInterface,
		"duration":        defaultCaptureDurationSecs,
		"filter":          defaultCaptureFilter,
		"maxPackets":      500,
		"maxBytes":        1048576, // 1 MiB safety cap
	})
	s.Require().NoError(err, "failed to enqueue PCAP task")

	// The capture runs for defaultCaptureDurationSecs seconds plus processing
	// overhead; give it 3× the capture window to complete.
	timeout := time.Duration(defaultCaptureDurationSecs*3) * time.Second
	result := s.pollResult(taskID, timeout)

	s.Require().True(result.Success,
		"PCAP task did not succeed (error_code=%d error_details=%q)",
		result.ErrorCode, result.ErrorDetails)

	// The action must publish at least one of: packet_count, byte_count, or
	// pcap_file (a URL/path to the captured data).  Any of these confirms the
	// capture ran and produced output.
	s.Require().NotEmpty(result.Outputs, "PCAP result outputs must not be empty")

	hasCaptureMeta := result.Outputs["packet_count"] != nil ||
		result.Outputs["byte_count"] != nil ||
		result.Outputs["pcap_file"] != nil

	s.Require().True(hasCaptureMeta,
		"expected at least one of packet_count/byte_count/pcap_file in outputs, got: %v",
		result.Outputs)
}

// TestPCAPRunCaptureWithDNSFilter enqueues a capture with a DNS-specific BPF
// filter.  npm-tools continuously runs dig queries, so there should always be
// DNS traffic to capture.
func (s *ekspcapSuite) TestPCAPRunCaptureWithDNSFilter() {
	taskID := uuid.New().String()

	err := s.Env().FakeIntake.Client().EnqueuePARTask(taskID, pcapActionFQN, map[string]interface{}{
		"interface": defaultCaptureInterface,
		"duration":  defaultCaptureDurationSecs,
		"filter":    "udp port 53",
		"maxPackets": 100,
	})
	s.Require().NoError(err, "failed to enqueue DNS-filter PCAP task")

	timeout := time.Duration(defaultCaptureDurationSecs*3) * time.Second
	result := s.pollResult(taskID, timeout)

	s.Require().True(result.Success,
		"DNS-filter PCAP task did not succeed (error_code=%d error_details=%q)",
		result.ErrorCode, result.ErrorDetails)

	s.Require().NotEmpty(result.Outputs, "PCAP result outputs must not be empty")
}

// TestPCAPRunCaptureShortTimeout issues a capture with a very short duration to
// verify the action honours the duration parameter and returns promptly.
func (s *ekspcapSuite) TestPCAPRunCaptureShortTimeout() {
	taskID := uuid.New().String()

	const shortDuration = 5 // seconds

	err := s.Env().FakeIntake.Client().EnqueuePARTask(taskID, pcapActionFQN, map[string]interface{}{
		"interface": defaultCaptureInterface,
		"duration":  shortDuration,
		"filter":    defaultCaptureFilter,
	})
	s.Require().NoError(err, "failed to enqueue short-duration PCAP task")

	// Allow up to 3× the requested duration for the result to arrive.
	timeout := time.Duration(shortDuration*3) * time.Second
	result := s.pollResult(taskID, timeout)

	s.Require().True(result.Success,
		"short-duration PCAP task did not succeed (error_code=%d error_details=%q)",
		result.ErrorCode, result.ErrorDetails)
}

// --- helpers ---

// pollResult is a convenience wrapper around GetPARTaskResult that integrates
// with the suite's require machinery.
func (s *ekspcapSuite) pollResult(taskID string, timeout time.Duration) *api.PARTaskResult {
	result, err := s.Env().FakeIntake.Client().GetPARTaskResult(taskID, timeout)
	s.Require().NoError(err, "timed out waiting for PAR task result (taskID=%s)", taskID)
	return result
}

// waitForPARReady blocks until:
//  1. The private-action-runner sidecar container inside the agent DaemonSet
//     pod reports Ready in Kubernetes.
//  2. PAR has made at least one call to the fakeintake dequeue endpoint,
//     confirming the polling loop is running.
func (s *ekspcapSuite) waitForPARReady() {
	selector := s.Env().Agent.LinuxNodeAgent.LabelSelectors["app"]

	// Step 1: wait for the PAR container to be Ready.
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
		assert.Fail(c, "private-action-runner container not yet ready")
	}, 5*time.Minute, 10*time.Second, "PAR container should become Ready within 5 minutes")

	// Step 2: confirm PAR is actively polling fakeintake.
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		count, err := s.Env().FakeIntake.Client().GetPARDequeueCount()
		assert.NoError(c, err)
		assert.Greater(c, count, 0, "PAR has not yet called the dequeue endpoint")
	}, 2*time.Minute, 3*time.Second, "PAR should start polling fakeintake within 2 minutes")
}
