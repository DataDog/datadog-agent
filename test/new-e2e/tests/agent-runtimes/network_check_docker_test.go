// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentruntimes

import (
	"testing"
	"time"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/dockeragentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awsdocker "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/docker"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"
)

const (
	networkCheckHostConfigPath      = "/tmp/network-check-e2e.yaml"
	networkCheckContainerConfigPath = "/etc/datadog-agent/conf.d/network.d/conf.yaml"
	networkCheckConfig              = `init_config:
instances:
  - collect_connection_state: true
    collect_connection_queues: true
    min_collection_interval: 5
`
)

type networkCheckDockerSuite struct {
	e2e.BaseSuite[environments.DockerHost]
}

func TestNetworkCheckDockerSuite(t *testing.T) {
	t.Parallel()

	e2e.Run(t, &networkCheckDockerSuite{}, e2e.WithProvisioner(awsdocker.Provisioner(
		awsdocker.WithRunOptions(
			ec2docker.WithPreAgentInstallHook(createNetworkCheckConfig),
			ec2docker.WithAgentOptions(
				dockeragentparams.WithExtraVolumes(networkCheckHostConfigPath+":"+networkCheckContainerConfigPath+":ro"),
			),
		),
	)))
}

func createNetworkCheckConfig(_ *aws.Environment, host *remote.Host) (pulumi.Resource, error) {
	return host.OS.FileManager().CopyInlineFile(
		pulumi.String(networkCheckConfig),
		networkCheckHostConfigPath,
	)
}

func (s *networkCheckDockerSuite) TestConnectionStateMetricsWithoutIPRoute2() {
	t := s.T()

	s.Env().RemoteHost.MustExecuteOn(t, `sudo docker exec datadog-agent sh -ec '
if dpkg-query -W iproute2 >/dev/null 2>&1; then
  echo "iproute2 is installed" >&2
  exit 1
fi
for binary in ss ip tc bridge; do
  if command -v "$binary" >/dev/null 2>&1; then
    echo "$binary is present at $(command -v "$binary")" >&2
    exit 1
  fi
done
'`)

	s.EventuallyWithT(func(c *assert.CollectT) {
		listeningMetrics, err := s.Env().FakeIntake.Client().FilterMetrics(
			"system.net.tcp4.listening",
			fakeintakeclient.WithMetricValueHigherThan(0),
		)
		require.NoError(c, err)
		require.NotEmpty(c, listeningMetrics, "network check did not report a listening TCP4 socket")

		queueMetrics, err := s.Env().FakeIntake.Client().FilterMetrics(
			"system.net.tcp.recv_q.count",
			fakeintakeclient.WithTags[*aggregator.MetricSeries]([]string{"state:listening"}),
			fakeintakeclient.WithMetricValueHigherThan(0),
		)
		require.NoError(c, err)
		require.NotEmpty(c, queueMetrics, "network check did not report listening-state receive queue samples")
	}, 5*time.Minute, 10*time.Second)
}
