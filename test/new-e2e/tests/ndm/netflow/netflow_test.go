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

	"github.com/DataDog/agent-payload/v5/gogen"
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
	"github.com/samber/lo"
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

// TestNetflow tests that the netflow-generator container is running and that the agent container
// is sending netflow data to the fakeintake
func (s *netflowDockerSuite) TestNetflow() {
	type ndmflowStats struct {
		flowTypes   map[string]int
		ipProtocols map[string]int
	}
	stats := ndmflowStats{make(map[string]int), make(map[string]int)}

	fakeintake := s.Env().FakeIntake.Client()
	s.EventuallyWithT(func(c *assert.CollectT) {
		// Check that the netflow-generator container is running and that the agent container is sending netflow data to the fakeintake
		metrics, err := fakeintake.FilterMetrics("datadog.netflow.aggregator.flows_flushed")
		assert.NoError(c, err)
		assert.Greater(c, len(metrics), 0, "no 'datadog.netflow.aggregator.flows_flushed' metrics yet")
		// Check value
		assert.NotEmptyf(c, lo.Filter(metrics[len(metrics)-1].GetPoints(), func(v *gogen.MetricPayload_MetricPoint, _ int) bool {
			return v.GetValue() > 0
		}), "No non-zero value of `datadog.netflow.aggregator.flows_flushed` in the last metric: %v", metrics[len(metrics)-1])

		// Validate that certain types of flows were received
		ndmflows, err := fakeintake.GetNDMFlows()
		assert.NoError(c, err)
		for _, ndmflow := range ndmflows {
			stats.flowTypes[ndmflow.FlowType]++
			stats.ipProtocols[ndmflow.IPProtocol]++
		}
		assert.Greater(c, stats.flowTypes["netflow5"], 0, "no netflow5 flows yet")
		assert.Greater(c, stats.ipProtocols["ICMP"], 0, "no ICMP flows yet")
		assert.Greater(c, stats.ipProtocols["UDP"], 0, "no UDP flows yet")
		assert.Greater(c, stats.ipProtocols["TCP"], 0, "no TCP flows yet")
	}, 5*time.Minute, 10*time.Second)
}
