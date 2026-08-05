// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package containers

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/dockeragentparams"
	scendocker "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awsdocker "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"
)

const noisyNeighborWorkloadName = "noisy-neighbor-pmu-workload"

type noisyNeighborPMUSuite struct {
	e2e.BaseSuite[environments.DockerHost]
}

func TestNoisyNeighborPMUSuite(t *testing.T) {
	t.Parallel()

	registry, _ := runner.GetProfile().ParamStore().GetWithDefault(parameters.ImagePullRegistry, "")
	image := "669783387624.dkr.ecr.us-east-1.amazonaws.com/dockerhub/library/busybox:1.37.0"
	if registry != "" {
		image = strings.SplitN(registry, ",", 2)[0] + "/dockerhub/library/busybox:1.37.0"
	}
	workload := fmt.Sprintf(`
services:
  noisy-neighbor-pmu-workload:
    image: %s
    container_name: %s
    cpuset: "0,1"
    command:
      - sh
      - -c
      - |
        while true; do
          taskset -p 1 $$$$ >/dev/null
          i=0; while [ $$i -lt 10000 ]; do i=$$((i + 1)); done
          taskset -p 2 $$$$ >/dev/null
          i=0; while [ $$i -lt 10000 ]; do i=$$((i + 1)); done
        done
`, image, noisyNeighborWorkloadName)

	agentOptions := []dockeragentparams.Option{
		// The DD_SYSTEM_PROBE_ prefix makes the Docker provisioner run the Agent privileged.
		dockeragentparams.WithAgentServiceEnvVariable("DD_SYSTEM_PROBE_ENABLED", pulumi.String("true")),
		dockeragentparams.WithAgentServiceEnvVariable("DD_NOISY_NEIGHBOR_ENABLED", pulumi.String("true")),
		dockeragentparams.WithAgentServiceEnvVariable("DD_NOISY_NEIGHBOR_PMU_METRICS_CPU_MIGRATIONS", pulumi.String("true")),
		dockeragentparams.WithExtraComposeManifest("noisy-neighbor-pmu-workload", pulumi.String(workload)),
	}
	e2e.Run(t, &noisyNeighborPMUSuite{}, e2e.WithProvisioner(awsdocker.Provisioner(
		awsdocker.WithRunOptions(scendocker.WithAgentOptions(agentOptions...)),
	)))
}

func (s *noisyNeighborPMUSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	defer s.CleanupOnSetupFailure()

	// A check instance is deliberately separate from system-probe enablement.
	s.Env().RemoteHost.MustExecute(`sudo docker exec datadog-agent sh -c "mkdir -p /etc/datadog-agent/conf.d/noisy_neighbor.d && printf 'init_config:\ninstances:\n  - {}\n' > /etc/datadog-agent/conf.d/noisy_neighbor.d/conf.yaml"`)
	s.Env().RemoteHost.MustExecute("sudo docker restart datadog-agent")
}

func (s *noisyNeighborPMUSuite) TestContainerCPUMigrations() {
	s.EventuallyWithT(func(collect *assert.CollectT) {
		metrics, err := s.Env().FakeIntake.Client().FilterMetrics(
			"noisy_neighbor.cpu_migrations",
			fakeintakeclient.WithTags[*aggregator.MetricSeries]([]string{"container_name:" + noisyNeighborWorkloadName}),
			fakeintakeclient.WithMetricValueHigherThan(0),
		)
		require.NoError(collect, err)
		require.NotEmpty(collect, metrics)
	}, 3*time.Minute, 10*time.Second)
}
