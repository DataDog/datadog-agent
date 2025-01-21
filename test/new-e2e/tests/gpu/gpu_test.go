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

type gpuSuite struct {
	e2e.BaseSuite[environments.Host]
	containerNameCounter int
}

const vectorAddDockerImg = "ghcr.io/datadog/apps-cuda-basic"

func dockerImageName() string {
	return fmt.Sprintf("%s:%s", vectorAddDockerImg, *imageTag)
}

// TestGPUSuite runs tests for the VM interface to ensure its implementation is correct.
// Not to be run in parallel, as some tests wait until the checks are available.
func TestGPUSuite(t *testing.T) {
	// incident-33572
	flake.Mark(t)
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

func (v *gpuSuite) requireGPUTags(metric *aggregator.MetricSeries) {
	foundRequiredTags := map[string]bool{
		"gpu_uuid":   false,
		"gpu_device": false,
		"gpu_vendor": false,
	}

	for _, tag := range metric.Tags {
		for requiredTag := range foundRequiredTags {
			if strings.HasPrefix(tag, requiredTag+":") {
				foundRequiredTags[requiredTag] = true
			}
		}
	}

	for requiredTag, found := range foundRequiredTags {
		v.Require().True(found, "required tag %s not found in %v", requiredTag, metric)
	}
}

func (v *gpuSuite) TestVectorAddProgramDetected() {
	flake.Mark(v.T())

	_ = v.runCudaDockerWorkload()

	v.EventuallyWithT(func(c *assert.CollectT) {
		// We are not including "gpu.memory", as that represents the "current
		// memory usage" and that might be zero at the time it's checked
		metricNames := []string{"gpu.utilization", "gpu.memory.max"}
		for _, metricName := range metricNames {
			metrics, err := v.Env().FakeIntake.Client().FilterMetrics(metricName, client.WithMetricValueHigherThan(0))
			assert.NoError(c, err)
			assert.Greater(c, len(metrics), 0, "no '%s' with value higher than 0 yet", metricName)

			for _, metric := range metrics {
				v.requireGPUTags(metric)
			}
		}
	}, 5*time.Minute, 10*time.Second)
}

func (v *gpuSuite) TestNvmlMetricsPresent() {
	// Nvml metrics are always being collected
	v.EventuallyWithT(func(c *assert.CollectT) {
		// Not all NVML metrics are supported in all devices. We check for some basic ones
		metricNames := []string{"gpu.temperature", "gpu.pci.throughput.tx", "gpu.power.usage"}
		for _, metricName := range metricNames {
			// We don't care about values, as long as the metrics are there. Values come from NVML
			// so we cannot control that.
			metrics, err := v.Env().FakeIntake.Client().FilterMetrics(metricName)
			assert.NoError(c, err)
			assert.Greater(c, len(metrics), 0, "no metric '%s' found")

			for _, metric := range metrics {
				v.requireGPUTags(metric)
			}
		}
	}, 5*time.Minute, 10*time.Second)
}

func (v *gpuSuite) TestWorkloadmetaHasGPUs() {
	out, err := v.Env().RemoteHost.Execute("sudo /opt/datadog-agent/bin/agent/agent workload-list")
	v.Require().NoError(err)
	v.Contains(out, "=== Entity gpu sources(merged):[runtime] id: ")
	if v.T().Failed() {
		v.T().Log(out)
	}
}
