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

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
)

var devMode = flag.Bool("devmode", false, "enable dev mode")

type gpuSuite struct {
	e2e.BaseSuite[environments.Host]
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

const vectorAddDockerImg = "nvcr.io/nvidia/k8s/cuda-sample:vectoradd-cuda10.2"
const gpuEnabledAMI = "ami-0f71e237bb2ba34be" // Ubuntu 22.04 with GPU drivers

// TestGPUSuite runs tests for the VM interface to ensure its implementation is correct.
func TestGPUSuite(t *testing.T) {
	// Marked as flaky pending removal of unattended-upgrades in the AMI
	flake.Mark(t)

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

	e2e.Run(t, &gpuSuite{}, suiteParams...)
}

func (v *gpuSuite) SetupSuite() {
	v.BaseSuite.SetupSuite()

	v.Env().RemoteHost.MustExecute(fmt.Sprintf("docker pull %s", vectorAddDockerImg))
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

func (v *gpuSuite) TestGPUCheckIsEnabled() {
	statusOutput := v.Env().Agent.Client.Status(agentclient.WithArgs([]string{"collector", "--json"}))

	var status collectorStatus
	err := json.Unmarshal([]byte(statusOutput.Content), &status)
	v.Require().NoError(err, "failed to unmarshal agent status")
	v.Require().Contains(status.RunnerStats.Checks, "gpu")

	gpuCheckStatus := status.RunnerStats.Checks["gpu"]
	v.Require().Equal(gpuCheckStatus.LastError, "")
}

func (v *gpuSuite) TestVectorAddProgramDetected() {
	vm := v.Env().RemoteHost

	out, err := vm.Execute(fmt.Sprintf("docker run --rm --gpus all %s", vectorAddDockerImg))
	v.Require().NoError(err)
	v.Require().NotEmpty(out)

	v.EventuallyWithT(func(c *assert.CollectT) {
		metricNames := []string{"gpu.memory", "gpu.utilization", "gpu.max_memory"}
		for _, metricName := range metricNames {
			metrics, err := v.Env().FakeIntake.Client().FilterMetrics(metricName, client.WithMetricValueHigherThan(0))
			assert.NoError(c, err)
			assert.Greater(c, len(metrics), 0, "no '%s' with value higher than 0 yet", metricName)
		}
	}, 5*time.Minute, 10*time.Second)
}
