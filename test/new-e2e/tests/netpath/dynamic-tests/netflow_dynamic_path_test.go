// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package networkpathdynamictests contains e2e tests for Network Path Dynamic Tests.
package networkpathdynamictests

import (
	_ "embed"
	"fmt"
	"net/netip"
	"path"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/dockeragentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"
	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"
)

//go:embed compose/netflow_dynamic_path.yaml
var netflowDynamicPathCompose string

//go:embed config/netflow_dynamic_path.yaml
var datadogYaml string

type netflowDynamicPathSuite struct {
	e2e.BaseSuite[environments.DockerHost]
}

type netflowTCPDestination struct {
	ip       string
	port     uint16
	flowType string
}

type netflowPathMatch struct {
	destination netflowTCPDestination
	path        *aggregator.Netpath
}

// TestNetflowDynamicPathSuite runs the Network Path Dynamic Tests suite backed by NetFlow traffic.
func TestNetflowDynamicPathSuite(t *testing.T) {
	e2e.Run(t, &netflowDynamicPathSuite{}, e2e.WithProvisioner(netflowDynamicPathProvisioner()))
}

func netflowDynamicPathProvisioner() provisioners.Provisioner {
	return provisioners.NewTypedPulumiProvisioner[environments.DockerHost]("", func(ctx *pulumi.Context, env *environments.DockerHost) error {
		name := "netflowdynamicpathvm"
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}

		host, err := ec2.NewVM(awsEnv, name, ec2.WithOS(e2eos.AmazonLinuxECSDefault))
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

		datadogYamlContent := fakeIntake.URL.ApplyT(func(url string) string {
			return strings.ReplaceAll(datadogYaml, "FAKEINTAKE_URL", url)
		}).(pulumi.StringOutput)

		configCommand, err := filemanager.CopyInlineFile(datadogYamlContent, path.Join(configPath, "datadog.yaml"),
			pulumi.DependsOn([]pulumi.Resource{createConfigDirCommand}))
		if err != nil {
			return err
		}

		dockerManager, err := docker.NewAWSManager(&awsEnv, host)
		if err != nil {
			return err
		}
		err = dockerManager.Export(ctx, &env.Docker.ManagerOutput)
		if err != nil {
			return err
		}

		envVars := pulumi.StringMap{"CONFIG_DIR": pulumi.String(configPath)}
		composeDependencies := []pulumi.Resource{configCommand}
		dockerAgent, err := agent.NewDockerAgent(&awsEnv, host, dockerManager,
			dockeragentparams.WithFakeintake(fakeIntake),
			dockeragentparams.WithAgentServiceEnvVariable("DD_SYSTEM_PROBE_ENABLED", pulumi.String("true")),
			dockeragentparams.WithAgentServiceEnvVariable("DD_TRACEROUTE_ENABLED", pulumi.String("true")),
			dockeragentparams.WithExtraComposeManifest("netflow-dynamic-path", pulumi.String(netflowDynamicPathCompose)),
			dockeragentparams.WithEnvironmentVariables(envVars),
			dockeragentparams.WithPulumiDependsOn(pulumi.DependsOn(composeDependencies)),
		)
		if err != nil {
			return err
		}
		dockerAgent.Export(ctx, &env.Agent.DockerAgentOutput)

		return nil
	}, nil)
}

func (s *netflowDynamicPathSuite) TestNetflowDynamicNetworkPath() {
	fakeintake := s.Env().FakeIntake.Client()

	var matched *netflowPathMatch
	s.EventuallyWithT(func(c *assert.CollectT) {
		ndmflows, err := fakeintake.GetNDMFlows()
		require.NoError(c, err)

		destinations := tcpDestinationsFromNDMFlows(ndmflows)
		require.NotEmpty(c, destinations, "no TCP NetFlow destinations with valid IP and port")

		assertMetricPresent(c, fakeintake, "datadog.netflow.aggregator.flows_flushed")
		assertAnyMetricPresent(c, fakeintake,
			"datadog.network_path.collector.schedule.pathtest_count",
			"datadog.network_path.collector.flush.pathtest_count",
		)

		netpaths, err := fakeintake.GetLatestNetpathEvents()
		require.NoError(c, err)
		require.NotEmpty(c, netpaths, "no network path events")

		match := findNetflowPathMatch(destinations, netpaths)
		require.NotNil(c, match, "no netflow-origin TCP network path matched an observed NDM flow destination")

		assert.Equal(c, payload.PathOriginNetflow, match.path.Origin)
		assert.Equal(c, payload.ProtocolTCP, match.path.Protocol)
		assert.Equal(c, payload.TestRunTypeDynamic, match.path.TestRunType)
		assert.Equal(c, payload.CollectorTypeAgent, match.path.CollectorType)
		assert.Equal(c, match.destination.ip, match.path.Destination.Hostname)
		assert.Equal(c, match.destination.port, match.path.Destination.Port)
		require.NotEmpty(c, match.path.Traceroute.Runs, "matched network path has no traceroute runs")
		assert.True(c, hasTracerouteHops(match.path), "matched network path has no traceroute hops")

		matched = match
	}, 5*time.Minute, 10*time.Second)

	if matched != nil {
		s.T().Logf("matched NetFlow %s destination %s:%d to netpath test_run_id=%s",
			matched.destination.flowType,
			matched.destination.ip,
			matched.destination.port,
			matched.path.TestRunID,
		)
	}
}

func tcpDestinationsFromNDMFlows(flows []*aggregator.NDMFlow) map[string]netflowTCPDestination {
	destinations := make(map[string]netflowTCPDestination)
	for _, flow := range flows {
		if flow == nil || flow.IPProtocol != "TCP" {
			continue
		}

		ip, err := netip.ParseAddr(flow.Destination.IP)
		if err != nil || !ip.Is4() {
			continue
		}

		port, err := strconv.ParseUint(flow.Destination.Port, 10, 16)
		if err != nil || port == 0 {
			continue
		}

		destination := netflowTCPDestination{
			ip:       ip.String(),
			port:     uint16(port),
			flowType: flow.FlowType,
		}
		destinations[destination.key()] = destination
	}
	return destinations
}

func findNetflowPathMatch(destinations map[string]netflowTCPDestination, netpaths []*aggregator.Netpath) *netflowPathMatch {
	for _, np := range netpaths {
		if np == nil || np.Origin != payload.PathOriginNetflow || np.Protocol != payload.ProtocolTCP {
			continue
		}

		destination, ok := destinations[destinationKey(np.Destination.Hostname, np.Destination.Port)]
		if !ok {
			continue
		}

		return &netflowPathMatch{
			destination: destination,
			path:        np,
		}
	}
	return nil
}

func hasTracerouteHops(np *aggregator.Netpath) bool {
	for _, run := range np.Traceroute.Runs {
		if len(run.Hops) > 0 {
			return true
		}
	}
	return false
}

func assertMetricPresent(c *assert.CollectT, fakeintake *fakeintakeclient.Client, metricName string) {
	metrics, err := fakeintake.FilterMetrics(metricName)
	require.NoError(c, err)
	assert.NotEmpty(c, metrics, "metric %q not found", metricName)
}

func assertAnyMetricPresent(c *assert.CollectT, fakeintake *fakeintakeclient.Client, metricNames ...string) {
	for _, metricName := range metricNames {
		metrics, err := fakeintake.FilterMetrics(metricName)
		require.NoError(c, err)
		if len(metrics) > 0 {
			return
		}
	}
	assert.Fail(c, fmt.Sprintf("none of the expected metrics were found: %v", metricNames))
}

func (d netflowTCPDestination) key() string {
	return destinationKey(d.ip, d.port)
}

func destinationKey(ip string, port uint16) string {
	return fmt.Sprintf("%s:%d", ip, port)
}
