// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package gpu contains e2e tests for the GPU monitoring module
package gpu

import (
	"encoding/json"
	"flag"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/fakeintake/client"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
)

var devMode = flag.Bool("devmode", false, "enable dev mode")
var imageTag = flag.String("image-tag", "main", "Docker image tag to use")
var mandatoryMetricTags = []string{"gpu_uuid", "gpu_device", "gpu_vendor"}

type gpuSuite struct {
	e2e.BaseSuite[environments.Host]
	containerNameCounter int
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

// TestGPUSuite runs tests for the VM interface to ensure its implementation is correct.
// Not to be run in parallel, as some tests wait until the checks are available.
func TestGPUSuite(t *testing.T) {
	// incident-33572. Pulumi seems to sometimes fail to create the stack with an error
	// we are not able to debug from the logs. We mark the test as flaky in that case only.
	flake.MarkOnLog(t, "error: an unhandled error occurred: waiting for RPCs:")
	provParams := getDefaultProvisionerParams()

	// Append our vectorAdd image for testing
	provParams.dockerImages = append(provParams.dockerImages, dockerImageName())

	provisioner := gpuInstanceProvisioner(provParams)

	suiteParams := []e2e.SuiteOption{e2e.WithProvisioner(provisioner)}
	if *devMode {
		suiteParams = append(suiteParams, e2e.WithDevMode())
	}

	suite := &gpuSuite{}

	e2e.Run(t, suite, suiteParams...)
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

// runCudaDockerWorkload runs a CUDA workload in a Docker container and returns the container ID
func (v *gpuSuite) runCudaDockerWorkload() string {
	// Configure some defaults
	vectorSize := 50000
	numLoops := 100      // Loop extra times to ensure the kernel runs for a bit
	waitTimeSeconds := 5 // Give enough time to our monitor to hook the probes
	binary := "/usr/local/bin/cuda-basic"
	containerName := fmt.Sprintf("cuda-basic-%d", v.containerNameCounter)
	v.containerNameCounter++

	cmd := fmt.Sprintf("docker run --gpus all --name %s %s %s %d %d %d", containerName, dockerImageName(), binary, vectorSize, numLoops, waitTimeSeconds)
	out, err := v.Env().RemoteHost.Execute(cmd)
	v.Require().NoError(err)
	v.Require().NotEmpty(out)

	v.T().Cleanup(func() {
		// Cleanup the container
		_, _ = v.Env().RemoteHost.Execute(fmt.Sprintf("docker rm -f %s", containerName))
	})

	containerIDCmd := fmt.Sprintf("docker inspect -f {{.Id}} %s", containerName)
	idOut, err := v.Env().RemoteHost.Execute(containerIDCmd)
	v.Require().NoError(err)
	v.Require().NotEmpty(idOut)

	return strings.TrimSpace(idOut)
}

func (v *gpuSuite) TestGPUCheckIsEnabled() {
	// Note that the GPU check should be enabled by autodiscovery, so it can take some time to be enabled
	v.EventuallyWithT(func(c *assert.CollectT) {
		statusOutput := v.Env().Agent.Client.Status(agentclient.WithArgs([]string{"collector", "--json"}))

		var status collectorStatus
		err := json.Unmarshal([]byte(statusOutput.Content), &status)

		assert.NoError(c, err, "failed to unmarshal agent status")
		assert.Contains(c, status.RunnerStats.Checks, "gpu")

		gpuCheckStatus := status.RunnerStats.Checks["gpu"]
		assert.Equal(c, gpuCheckStatus.LastError, "")
	}, 2*time.Minute, 10*time.Second)
}

func (v *gpuSuite) TestGPUSysprobeEndpointIsResponding() {
	v.EventuallyWithT(func(c *assert.CollectT) {
		out, err := v.Env().RemoteHost.Execute("sudo curl -s --unix /opt/datadog-agent/run/sysprobe.sock http://unix/gpu/check")
		assert.NoError(c, err)
		assert.NotEmpty(c, out)
	}, 2*time.Minute, 10*time.Second)
}

func (v *gpuSuite) TestVectorAddProgramDetected() {
	flake.Mark(v.T())

	_ = v.runCudaDockerWorkload()

	v.EventuallyWithT(func(c *assert.CollectT) {
		// We are not including "gpu.memory", as that represents the "current
		// memory usage" and that might be zero at the time it's checked
		metricNames := []string{"gpu.utilization"}
		for _, metricName := range metricNames {
			metrics, err := v.Env().FakeIntake.Client().FilterMetrics(metricName, client.WithMetricValueHigherThan(0), client.WithMatchingTags[*aggregator.MetricSeries](mandatoryMetricTagRegexes()))
			assert.NoError(c, err)
			assert.Greater(c, len(metrics), 0, "no '%s' with value higher than 0 yet", metricName)
		}
	}, 5*time.Minute, 10*time.Second)
}

func (v *gpuSuite) TestNvmlMetricsPresent() {
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

			metrics, err := v.Env().FakeIntake.Client().FilterMetrics(metric.name, options...)
			assert.NoError(c, err)

			assert.Greater(c, len(metrics), 0, "no metric '%s' found", metric.name)
		}
	}, 5*time.Minute, 10*time.Second)
}

func (v *gpuSuite) TestWorkloadmetaHasGPUs() {
	var out string
	// Wait until our collector has ran and we have GPUs in the workloadmeta. We don't have exact control on the timing of execution
	v.EventuallyWithT(func(c *assert.CollectT) {
		var err error
		out, err = v.Env().RemoteHost.Execute("sudo /opt/datadog-agent/bin/agent/agent workload-list")
		assert.NoError(c, err)
		assert.Contains(c, out, "=== Entity gpu sources(merged):[runtime] id: ")
	}, 30*time.Second, 1*time.Second)

	if v.T().Failed() {
		// log the output for debugging in case of failure
		v.T().Log(out)
	}
}
