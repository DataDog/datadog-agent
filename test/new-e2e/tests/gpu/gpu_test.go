// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package gpu contains e2e tests for the GPU monitoring module
package gpu

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/fakeintake/client"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
)

var devMode = flag.Bool("devmode", false, "enable dev mode")
var imageTag = flag.String("image-tag", "main", "Docker image tag to use")
var mandatoryMetricTags = []string{"gpu_uuid", "gpu_device", "gpu_vendor", "gpu_driver_version"}

type gpuBaseSuite[Env any] struct {
	e2e.BaseSuite[Env]
	caps                     suiteCapabilities
	agentRestartsAtSuiteInit map[agentComponent]int
	provisioner              provisioners.Provisioner
}

const vectorAddDockerImg = "ghcr.io/datadog/apps-cuda-basic"

func dockerImageName() string {
	return fmt.Sprintf("%s:%s", vectorAddDockerImg, *imageTag)
}

func mandatoryMetricTagRegexes() []*regexp.Regexp {
	regexes := make([]*regexp.Regexp, 0, len(mandatoryMetricTags))
	for _, tag := range mandatoryMetricTags {
		regexes = append(regexes, regexp.MustCompile(fmt.Sprintf("%s:.*", tag)))
	}

	return regexes
}

type gpuHostSuite struct {
	gpuBaseSuite[environments.Host]
}

// TestGPUHostSuite runs tests for the VM interface to ensure its implementation is correct.
// Not to be run in parallel, as some tests wait until the checks are available.
func TestGPUHostSuite(t *testing.T) {
	// incident-33572. Pulumi seems to sometimes fail to create the stack with an error
	// we are not able to debug from the logs. We mark the test as flaky in that case only.
	flake.MarkOnLog(t, "error: an unhandled error occurred: waiting for RPCs:")
	provParams := getDefaultProvisionerParams()

	// Append our vectorAdd image for testing
	provParams.dockerImages = append(provParams.dockerImages, dockerImageName())

	provisioner := gpuHostProvisioner(provParams)

	suiteParams := []e2e.SuiteOption{e2e.WithProvisioner(provisioner)}
	if *devMode {
		suiteParams = append(suiteParams, e2e.WithDevMode())
	}

	suite := &gpuHostSuite{}

	e2e.Run(t, suite, suiteParams...)
}

func (s *gpuHostSuite) SetupSuite() {
	// The base suite needs the capabilities struct, so set it before calling the base SetupSuite
	s.caps = &hostCapabilities{&s.BaseSuite}
	s.gpuBaseSuite.SetupSuite()
}

type gpuK8sSuite struct {
	gpuBaseSuite[environments.Kubernetes]
}

// TestGPUK8sSuite runs tests for the VM interface to ensure its implementation is correct.
// Not to be run in parallel, as some tests wait until the checks are available.
func TestGPUK8sSuite(t *testing.T) {
	// incident-33572. Pulumi seems to sometimes fail to create the stack with an error
	// we are not able to debug from the logs. We mark the test as flaky in that case only.
	flake.MarkOnLog(t, "error: an unhandled error occurred: waiting for RPCs:")

	// Temporary fix while we debug the issue
	flake.MarkOnLog(t, "panic: Expected to find a single pod")

	// Nvidia GPU operator images are not mirrored in our private registries, so ensure
	// we're not breaking main if we get rate limited
	flake.MarkOnLog(t, "rate limit")
	provParams := getDefaultProvisionerParams()

	// Append our vectorAdd image for testing
	provParams.dockerImages = append(provParams.dockerImages, dockerImageName())

	provisioner := gpuK8sProvisioner(provParams)

	suiteParams := []e2e.SuiteOption{e2e.WithProvisioner(provisioner)}
	if *devMode {
		suiteParams = append(suiteParams, e2e.WithDevMode())
	}

	suite := &gpuK8sSuite{
		gpuBaseSuite: gpuBaseSuite[environments.Kubernetes]{
			provisioner: provisioner,
		},
	}

	e2e.Run(t, suite, suiteParams...)
}

func (s *gpuK8sSuite) SetupSuite() {
	// The base suite needs the capabilities struct, so set it before calling the base SetupSuite
	s.caps = &kubernetesCapabilities{&s.BaseSuite}
	s.gpuBaseSuite.SetupSuite()
}

// TODO: Extract this to common package? service_discovery uses it too
type checkStatus struct {
	CheckID           string `json:"CheckID"`
	CheckName         string `json:"CheckName"`
	CheckConfigSource string `json:"CheckConfigSource"`
	ExecutionTimes    []int  `json:"ExecutionTimes"`
	LastError         string `json:"LastError"`
}

type runnerStats struct {
	Checks map[string]checkStatus `json:"Checks"`
}

type collectorStatus struct {
	RunnerStats runnerStats `json:"runnerStats"`
}

func (v *gpuBaseSuite[Env]) SetupSuite() {
	v.BaseSuite.SetupSuite()
	v.agentRestartsAtSuiteInit = make(map[agentComponent]int)

	// Get initial agent service restart counts
	services := []agentComponent{agentComponentCoreAgent, agentComponentSystemProbe}
	for _, service := range services {
		v.agentRestartsAtSuiteInit[service] = v.caps.GetRestartCount(service)
	}
}

func (s *gpuK8sSuite) AfterTest(suiteName, testName string) {
	s.BaseSuite.AfterTest(suiteName, testName)

	if !s.T().Failed() {
		return
	}

	k8sPulumiProvisioner, ok := s.provisioner.(*provisioners.PulumiProvisioner[environments.Kubernetes])
	if !ok {
		return
	}

	diagnose, err := k8sPulumiProvisioner.Diagnose(context.Background(), s.Env().KubernetesCluster.ClusterName)
	if err != nil {
		s.T().Logf("failed to diagnose provisioner: %v", err)
	}

	s.T().Logf("Diagnose result:\n\n%s", diagnose)
}

// runCudaDockerWorkload runs a CUDA workload in a Docker container and returns the container ID
func (v *gpuBaseSuite[Env]) runCudaDockerWorkload() string {
	// Configure some defaults
	vectorSize := 50000
	numLoops := 100      // Loop extra times to ensure the kernel runs for a bit
	waitTimeSeconds := 5 // Give enough time to our monitor to hook the probes
	binary := "/usr/local/bin/cuda-basic"
	args := []string{binary, strconv.Itoa(vectorSize), strconv.Itoa(numLoops), strconv.Itoa(waitTimeSeconds)}

	containerID, err := v.caps.RunContainerWorkloadWithGPUs(dockerImageName(), args...)
	v.Require().NoError(err)
	v.Require().NotEmpty(containerID)

	return containerID
}

func (v *gpuBaseSuite[Env]) TestGPUCheckIsEnabled() {
	// Note that the GPU check should be enabled by autodiscovery, so it can take some time to be enabled
	v.EventuallyWithT(func(c *assert.CollectT) {
		statusOutput := v.caps.Agent().Status(agentclient.WithArgs([]string{"collector", "--json"}))

		// Keep only the second-to-last line of the output, which is the JSON status. The rest is standard error
		// TODO: Make the status command return stdout/stderr separately
		statusLines := strings.Split(statusOutput.Content, "\n")
		assert.Greater(c, len(statusLines), 1, "status output should have at least 2 lines")
		jsonStatus := statusLines[len(statusLines)-2]

		var status collectorStatus
		err := json.Unmarshal([]byte(jsonStatus), &status)

		assert.NoError(c, err, "failed to unmarshal agent status")
		assert.Contains(c, status.RunnerStats.Checks, "gpu")

		gpuCheckStatus := status.RunnerStats.Checks["gpu"]
		assert.Equal(c, gpuCheckStatus.LastError, "")
	}, 2*time.Minute, 10*time.Second)
}

func (v *gpuBaseSuite[Env]) TestGPUSysprobeEndpointIsResponding() {
	v.EventuallyWithT(func(c *assert.CollectT) {
		out, err := v.caps.QuerySysprobe("gpu/check")
		assert.NoError(c, err)
		assert.NotEmpty(c, out)
	}, 2*time.Minute, 10*time.Second)
}

func (v *gpuBaseSuite[Env]) TestLimitMetricsAreReported() {
	v.EventuallyWithT(func(c *assert.CollectT) {
		metricNames := []string{"gpu.core.limit", "gpu.memory.limit"}
		for _, metricName := range metricNames {
			metrics, err := v.caps.FakeIntake().Client().FilterMetrics(metricName, client.WithMetricValueHigherThan(0), client.WithMatchingTags[*aggregator.MetricSeries](mandatoryMetricTagRegexes()))
			assert.NoError(c, err)
			assert.Greater(c, len(metrics), 0, "no '%s' with value higher than 0 yet", metricName)
		}
	}, 5*time.Minute, 10*time.Second)
}

func (v *gpuBaseSuite[Env]) TestVectorAddProgramDetected() {
	_ = v.runCudaDockerWorkload()

	v.EventuallyWithT(func(c *assert.CollectT) {
		// We are not including "gpu.memory", as that represents the "current
		// memory usage" and that might be zero at the time it's checked
		metricNames := []string{"gpu.core.usage"}

		var usageMetricTags []string
		for _, metricName := range metricNames {
			metrics, err := v.caps.FakeIntake().Client().FilterMetrics(metricName, client.WithMetricValueHigherThan(0), client.WithMatchingTags[*aggregator.MetricSeries](mandatoryMetricTagRegexes()))
			assert.NoError(c, err)
			assert.Greater(c, len(metrics), 0, "no '%s' with value higher than 0 yet", metricName)

			if metricName == "gpu.core.usage" && len(metrics) > 0 {
				usageMetricTags = metrics[0].Tags
			}
		}

		if len(usageMetricTags) > 0 {
			// Ensure we get the limit metric with the same tags as the usage one
			limitMetrics, err := v.caps.FakeIntake().Client().FilterMetrics("gpu.core.limit", client.WithTags[*aggregator.MetricSeries](usageMetricTags))
			assert.NoError(c, err)
			assert.Greater(c, len(limitMetrics), 0, "no 'gpu.core.limit' with tags %v", usageMetricTags)
		}
	}, 5*time.Minute, 10*time.Second)
}

func (v *gpuBaseSuite[Env]) TestNvmlMetricsPresent() {
	// Nvml metrics are always being collected
	v.EventuallyWithT(func(c *assert.CollectT) {
		// Not all NVML metrics are supported in all devices. We check for some basic ones
		metrics := []struct {
			name           string
			deviceSpecific bool
		}{
			{"gpu.temperature", true},
			{"gpu.pci.throughput.tx", true},
			{"gpu.power.usage", true},
			{"gpu.device.total", false},
		}
		for _, metric := range metrics {
			// We don't care about values, as long as the metrics are there. Values come from NVML
			// so we cannot control that.
			var options []client.MatchOpt[*aggregator.MetricSeries]
			if metric.deviceSpecific {
				// device-specific metrics should be tagged with device tags
				options = append(options, client.WithMatchingTags[*aggregator.MetricSeries](mandatoryMetricTagRegexes()))
			}

			metrics, err := v.caps.FakeIntake().Client().FilterMetrics(metric.name, options...)
			assert.NoError(c, err)

			assert.Greater(c, len(metrics), 0, "no metric '%s' found", metric.name)
		}
	}, 5*time.Minute, 10*time.Second)
}

func (v *gpuBaseSuite[Env]) TestWorkloadmetaHasGPUs() {
	var out string
	// Wait until our collector has ran and we have GPUs in the workloadmeta. We don't have exact control on the timing of execution
	v.EventuallyWithT(func(c *assert.CollectT) {
		status, err := v.caps.Agent().WorkloadList()
		assert.NoError(c, err)
		out = status.Content
		assert.Contains(c, out, "=== Entity gpu sources(merged):[runtime] id: ")
	}, 30*time.Second, 1*time.Second)

	if v.T().Failed() {
		// log the output for debugging in case of failure
		v.T().Log(out)
	}
}

// TestZZAgentDidNotRestart checks that the agent did not restart during the test suite
// Add zz to name to run this test last, as we want to run it after all other tests have run
// to ensure that no restarts happened during the test suite, which would be an error that we
// might not catch with the test themselves (e.g., we send correct metrics and then we panic)
func (v *gpuBaseSuite[Env]) TestZZAgentDidNotRestart() {
	for service, initialCount := range v.agentRestartsAtSuiteInit {
		finalCount := v.caps.GetRestartCount(service)
		v.Assert().Equal(initialCount, finalCount, "Service %s restarted during test suite", service)
	}
}
