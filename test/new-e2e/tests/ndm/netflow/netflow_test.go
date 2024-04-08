// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package netflow contains e2e tests for netflow
package netflow

import (
	_ "embed"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/components/datadog/dockeragentparams"
	"github.com/DataDog/test-infra-definitions/components/docker"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/fakeintake"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

//go:embed compose/netflowCompose.yaml
var netflowCompose string

//go:embed config/netflowConfig.yaml
var datadogYaml string

// netflowDockerProvisioner defines a stack with a docker agent on an AmazonLinuxECS VM
// with the netflow-generator running and sending netflow payloads to the agent
func netflowDockerProvisioner() e2e.Provisioner {
	return e2e.NewTypedPulumiProvisioner[environments.DockerHost]("", func(ctx *pulumi.Context, env *environments.DockerHost) error {
		name := "netflowvm"
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}

		host, err := ec2.NewVM(awsEnv, name, ec2.WithOS(os.AmazonLinuxECSDefault))
		if err != nil {
			return err
		}
		host.Export(ctx, &env.RemoteHost.HostOutput)

		fakeIntake, err := fakeintake.NewECSFargateInstance(awsEnv, name)
		if err != nil {
			return err
		}
		fakeIntake.Export(ctx, &env.FakeIntake.FakeintakeOutput)

		filemanager := host.OS.FileManager()

		createConfigDirCommand, configPath, err := filemanager.TempDirectory("config")
		if err != nil {
			return err
		}

		// update agent datadog.yaml config content at fakeintake address resolution
		datadogYamlContent := fakeIntake.URL.ApplyT(func(url string) string {
			return strings.ReplaceAll(datadogYaml, "FAKEINTAKE_URL", url)
		}).(pulumi.StringOutput)

		dontUseSudo := false
		configCommand, err := filemanager.CopyInlineFile(datadogYamlContent, path.Join(configPath, "datadog.yaml"), dontUseSudo,
			pulumi.DependsOn([]pulumi.Resource{createConfigDirCommand}))
		if err != nil {
			return err
		}

		dockerManager, _, err := docker.NewManager(*awsEnv.CommonEnvironment, host)
		if err != nil {
			return err
		}

		envVars := pulumi.StringMap{"CONFIG_DIR": pulumi.String(configPath)}
		composeDependencies := []pulumi.Resource{configCommand}
		dockerAgent, err := agent.NewDockerAgent(*awsEnv.CommonEnvironment, host, dockerManager,
			dockeragentparams.WithFakeintake(fakeIntake),
			dockeragentparams.WithExtraComposeManifest("netflow-generator", pulumi.String(netflowCompose)),
			dockeragentparams.WithEnvironmentVariables(envVars),
			dockeragentparams.WithPulumiDependsOn(pulumi.DependsOn(composeDependencies)),
		)
		if err != nil {
			return err
		}
		dockerAgent.Export(ctx, &env.Agent.DockerAgentOutput)

		return err
	}, nil)
}

type netflowDockerSuite struct {
	e2e.BaseSuite[environments.DockerHost]
}

// TestNetflowSuite runs the netflow e2e suite
func TestNetflowSuite(t *testing.T) {
	e2e.Run(t, &netflowDockerSuite{}, e2e.WithProvisioner(netflowDockerProvisioner()))
}

// TestNetflow does some basic validation to confirm that the netflow-generator container is running and sending netflow data to the agent,
// and that the agent container is sending netflow payloads to the fakeintake.
func (s *netflowDockerSuite) TestNetflow() {
	const minFlows = 100
	fakeintake := s.Env().FakeIntake.Client()
	s.EventuallyWithT(func(c *assert.CollectT) {
		// Validate that flows_flushed metric(s) have been sent.
		metrics, err := fakeintake.FilterMetrics("datadog.netflow.aggregator.flows_flushed")
		assert.NoError(c, err)
		assert.Greater(c, len(metrics), 0, "no 'datadog.netflow.aggregator.flows_flushed' metrics")

		// Check that the sum of flows_flushed metrics >= minFlows.
		var totalFlowsFlushed float64
		for _, metric := range metrics {
			for _, point := range metric.GetPoints() {
				totalFlowsFlushed += point.GetValue()
			}
		}
		assert.GreaterOrEqual(c, int(totalFlowsFlushed), minFlows, "did not receive >= %d 'datadog.netflow.aggregator.flows_flushed' metrics", minFlows)

		type ndmflowStats struct {
			flowTypes   map[string]int
			ipProtocols map[string]int
		}
		stats := ndmflowStats{make(map[string]int), make(map[string]int)}

		ndmflows, err := fakeintake.GetNDMFlows()
		assert.NoError(c, err)
		// Validate that the number netflow payloads sent by the agent and received by the fakeintake >= minFlows.
		assert.GreaterOrEqual(c, len(ndmflows), minFlows, "did not receive >= %d netflow payloads", minFlows)

		// The netflow-generator workload app sends netflow5 payloads for various types of flows.  Validate that the expected types of flows were received.
		for _, ndmflow := range ndmflows {
			stats.flowTypes[ndmflow.FlowType]++
			stats.ipProtocols[ndmflow.IPProtocol]++
		}
		s.T().Logf("flows_flushed metric shows that agent sent %d flows, fakeintake received %d ndmflows", int(totalFlowsFlushed), len(ndmflows))
		s.T().Logf("stats: %+v", stats)
		assert.Greater(c, stats.flowTypes["netflow5"], 0, "no netflow5 flows yet")
		assert.Greater(c, stats.ipProtocols["ICMP"], 0, "no ICMP flows yet")
		assert.Greater(c, stats.ipProtocols["UDP"], 0, "no UDP flows yet")
		assert.Greater(c, stats.ipProtocols["TCP"], 0, "no TCP flows yet")
	}, 5*time.Minute, 10*time.Second)
}
