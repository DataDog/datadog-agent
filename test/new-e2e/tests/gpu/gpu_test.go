// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package gpu

import (
	"encoding/json"
	"flag"
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/fakeintake/client"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/components/os"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
)

var devMode = flag.Bool("devmode", false, "enable dev mode")
var imageTag = flag.String("image-tag", "main", "Docker image tag to use")

type gpuSuite struct {
	e2e.BaseSuite[environments.Host]
	imageTag string
}

const defaultGpuCheckConfig = `
init_config:
  min_collection_interval: 5

instances:
  - {}
`

const defaultSysprobeConfig = `
gpu_monitoring:
  enabled: true
`

const vectorAddDockerImg = "ghcr.io/datadog/apps-cuda-basic"
const gpuEnabledAMI = "ami-0f71e237bb2ba34be" // Ubuntu 22.04 with GPU drivers

// TestGPUSuite runs tests for the VM interface to ensure its implementation is correct.
// Not to be run in parallel, as some tests wait until the checks are available.
func TestGPUSuite(t *testing.T) {
	provisioner := awshost.Provisioner(
		awshost.WithEC2InstanceOptions(
			ec2.WithInstanceType("g4dn.xlarge"),
			ec2.WithAMI(gpuEnabledAMI, os.Ubuntu2204, os.AMD64Arch),
		),
		awshost.WithAgentOptions(
			agentparams.WithIntegration("gpu.d", defaultGpuCheckConfig),
			agentparams.WithSystemProbeConfig(defaultSysprobeConfig),
		),
		awshost.WithDocker(),
	)

	suiteParams := []e2e.SuiteOption{e2e.WithProvisioner(provisioner)}
	if *devMode {
		suiteParams = append(suiteParams, e2e.WithDevMode())
	}

	suite := &gpuSuite{
		imageTag: *imageTag,
	}

	e2e.Run(t, suite, suiteParams...)
}

func (v *gpuSuite) SetupSuite() {
	v.BaseSuite.SetupSuite()

	v.Env().RemoteHost.MustExecute(fmt.Sprintf("docker pull %s", v.dockerImageName()))
}

func (v *gpuSuite) dockerImageName() string {
	return fmt.Sprintf("%s:%s", vectorAddDockerImg, v.imageTag)
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

func (v *gpuSuite) runCudaDockerWorkload() {
	// Configure some defaults
	vectorSize := 50000
	numLoops := 100      // Loop extra times to ensure the kernel runs for a bit
	waitTimeSeconds := 5 // Give enough time to our monitor to hook the probes
	binary := "/usr/local/bin/cuda-basic"

	cmd := fmt.Sprintf("docker run --rm --gpus all %s %s %d %d %d", v.dockerImageName(), binary, vectorSize, numLoops, waitTimeSeconds)
	out, err := v.Env().RemoteHost.Execute(cmd)
	v.Require().NoError(err)
	v.Require().NotEmpty(out)
}

func (v *gpuSuite) TestGPUCheckIsEnabled() {
	statusOutput := v.Env().Agent.Client.Status(agentclient.WithArgs([]string{"collector", "--json"}))

	var status collectorStatus
	err := json.Unmarshal([]byte(statusOutput.Content), &status)
	v.Require().NoError(err, "failed to unmarshal agent status")
	v.Require().Contains(status.RunnerStats.Checks, "gpu")

	gpuCheckStatus := status.RunnerStats.Checks["gpu"]
	v.Require().Equal(gpuCheckStatus.LastError, "")
}

func (v *gpuSuite) TestGPUSysprobeEndpointIsResponding() {
	v.EventuallyWithT(func(c *assert.CollectT) {
		out, err := v.Env().RemoteHost.Execute("sudo curl -s --unix /opt/datadog-agent/run/sysprobe.sock http://unix/gpu/check")
		assert.NoError(c, err)
		assert.NotEmpty(c, out)
	}, 2*time.Minute, 10*time.Second)
}

func (v *gpuSuite) TestVectorAddProgramDetected() {
	v.runCudaDockerWorkload()

	v.EventuallyWithT(func(c *assert.CollectT) {
		// We are not including "gpu.memory", as that represents the "current
		// memory usage" and that might be zero at the time it's checked
		metricNames := []string{"gpu.utilization", "gpu.memory.max"}
		for _, metricName := range metricNames {
			metrics, err := v.Env().FakeIntake.Client().FilterMetrics(metricName, client.WithMetricValueHigherThan(0))
			assert.NoError(c, err)
			assert.Greater(c, len(metrics), 0, "no '%s' with value higher than 0 yet", metricName)
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
		}
	}, 5*time.Minute, 10*time.Second)
}
