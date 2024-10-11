// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package gpu

import (
	"flag"
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"

	"github.com/DataDog/datadog-agent/test/fakeintake/client"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
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

// TestGPUSuite runs tests for the VM interface to ensure its implementation is correct.
func TestGPUSuite(t *testing.T) {
	provisioner := awshost.Provisioner(
		awshost.WithEC2InstanceOptions(ec2.WithInstanceType("g4dn.xlarge")),
		awshost.WithAgentOptions(
			agentparams.WithIntegration("gpu", defaultGpuCheckConfig),
			agentparams.WithSystemProbeConfig(defaultSysprobeConfig),
		),
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
