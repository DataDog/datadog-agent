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
var netflowDynamicPathDatadogYaml string

//go:embed config/netflow_dynamic_path_disabled.yaml
var netflowDynamicPathDisabledDatadogYaml string

const (
	expectedNetflowFlowType  = "netflow5"
	expectedNetflowNamespace = "e2e-netflow"

	selfSourceNetflowDestinationIP   = "8.8.8.8"
	selfSourceNetflowDestinationPort = uint16(32749)
	netflowAgentSourceSkippedMetric  = "datadog.network_path.collector.schedule.conns_skipped"
	netflowAgentSourceSkippedReason  = "reason:skip_netflow_agent_source"
)

const sendSelfSourceNetflowScript = `import socket
import struct
import sys
import time

src_ip = sys.argv[1]
dst_ip = sys.argv[2]
dst_port = int(sys.argv[3])

sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
for seq in range(20):
    now = int(time.time())
    header = struct.pack("!HHIIIIBBH", 5, 1, 100000, now, 0, seq, 0, 0, 0)
    record = struct.pack(
        "!4s4s4sHHIIIIHHBBBBHHBBH",
        socket.inet_aton(src_ip),
        socket.inet_aton(dst_ip),
        b"\x00\x00\x00\x00",
        0,
        0,
        10 + seq,
        500 + seq,
        90000,
        100000,
        40000 + seq,
        dst_port,
        0,
        0x10,
        6,
        0,
        0,
        0,
        32,
        32,
        0,
    )
    sock.sendto(header + record, ("127.0.0.1", 2056))
    time.sleep(0.2)
`

type netflowDynamicPathSuite struct {
	e2e.BaseSuite[environments.DockerHost]
}

type netflowDynamicPathDisabledSuite struct {
	e2e.BaseSuite[environments.DockerHost]
}

type netflowTCPDestination struct {
	ip        string
	port      uint16
	flowType  string
	namespace string
}

type netflowPathMatch struct {
	destination netflowTCPDestination
	path        *aggregator.Netpath
}

// TestNetflowDynamicPathSuite runs the Network Path Dynamic Tests suite backed by NetFlow traffic.
func TestNetflowDynamicPathSuite(t *testing.T) {
	e2e.Run(t, &netflowDynamicPathSuite{}, e2e.WithProvisioner(netflowDynamicPathProvisioner(netflowDynamicPathDatadogYaml)))
}

// TestNetflowDynamicPathDisabledSuite verifies NetFlow payloads do not schedule dynamic paths when disabled.
func TestNetflowDynamicPathDisabledSuite(t *testing.T) {
	e2e.Run(t, &netflowDynamicPathDisabledSuite{}, e2e.WithProvisioner(netflowDynamicPathProvisioner(netflowDynamicPathDisabledDatadogYaml)))
}

func netflowDynamicPathProvisioner(datadogYaml string) provisioners.Provisioner {
	return provisioners.NewTypedPulumiProvisioner[environments.DockerHost]("", func(ctx *pulumi.Context, env *environments.DockerHost) error {
		name := "netflowdynamicpathvm"
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}

		host, err := ec2.NewVM(awsEnv, name, ec2.WithOS(e2eos.UbuntuDefault))
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
		assertMetricPresent(c, fakeintake, "datadog.network_path.collector.schedule.pathtest_count")
		assertMetricPresent(c, fakeintake, "datadog.network_path.collector.flush.pathtest_count")

		netpaths, err := fakeintake.GetLatestNetpathEvents()
		require.NoError(c, err)
		require.NotEmpty(c, netpaths, "no network path events")

		match := findNetflowPathMatch(destinations, netpaths)
		require.NotNil(c, match, "no netflow-origin TCP network path matched an observed NDM flow destination")

		assert.Equal(c, expectedNetflowFlowType, match.destination.flowType)
		assert.Equal(c, expectedNetflowNamespace, match.destination.namespace)
		assert.Equal(c, match.destination.namespace, match.path.Namespace)
		assert.Equal(c, payload.PathOriginNetflow, match.path.Origin)
		assert.Equal(c, payload.SourceProductNetflow, match.path.SourceProduct)
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

func (s *netflowDynamicPathSuite) TestNetflowDynamicPathSkipsAgentSource() {
	fakeintake := s.Env().FakeIntake.Client()
	agentIP := s.agentContainerIPv4()

	s.stopNetflowGenerator()
	defer s.startNetflowGenerator()

	require.NoError(s.T(), fakeintake.FlushServerAndResetAggregators())
	s.sendSelfSourceNetflow(agentIP)

	var matchedFlow *aggregator.NDMFlow
	s.EventuallyWithT(func(c *assert.CollectT) {
		ndmflows, err := fakeintake.GetNDMFlows()
		require.NoError(c, err)

		match := findSelfSourceNDMFlow(ndmflows, agentIP.String(), selfSourceNetflowDestinationIP, selfSourceNetflowDestinationPort)
		require.NotNil(c, match, "self-source NetFlow row was not received by fakeintake")

		present, err := metricPresentWithTags(
			fakeintake,
			netflowAgentSourceSkippedMetric,
			[]string{netflowAgentSourceSkippedReason},
		)
		require.NoError(c, err)
		assert.True(c, present, "self-source NetFlow skip metric was not received by fakeintake")

		matchedFlow = match
	}, 2*time.Minute, 10*time.Second)

	require.Never(s.T(), func() bool {
		netpaths, err := fakeintake.GetLatestNetpathEvents()
		if err != nil {
			s.T().Logf("error querying netpath events: %v", err)
			return true
		}
		match := findNetflowPathByDestination(netpaths, selfSourceNetflowDestinationIP, selfSourceNetflowDestinationPort)
		if match == nil {
			return false
		}

		s.T().Logf("unexpected self-source netflow network path event: destination=%s:%d test_run_id=%s",
			match.Destination.Hostname,
			match.Destination.Port,
			match.TestRunID,
		)
		return true
	}, 1*time.Minute, 10*time.Second, "self-source NetFlow row produced a network path event")

	if matchedFlow != nil {
		s.T().Logf("self-source NetFlow row source=%s destination=%s:%s was skipped by Network Path",
			matchedFlow.Source.IP,
			matchedFlow.Destination.IP,
			matchedFlow.Destination.Port,
		)
	}
}

func (s *netflowDynamicPathDisabledSuite) TestNetflowDynamicNetworkPathDisabled() {
	fakeintake := s.Env().FakeIntake.Client()

	s.EventuallyWithT(func(c *assert.CollectT) {
		ndmflows, err := fakeintake.GetNDMFlows()
		require.NoError(c, err)

		destinations := tcpDestinationsFromNDMFlows(ndmflows)
		require.NotEmpty(c, destinations, "no TCP NetFlow destinations with valid IP and port")

		assertMetricPresent(c, fakeintake, "datadog.netflow.aggregator.flows_flushed")
	}, 2*time.Minute, 10*time.Second)

	require.NoError(s.T(), fakeintake.FlushServerAndResetAggregators())

	s.EventuallyWithT(func(c *assert.CollectT) {
		ndmflows, err := fakeintake.GetNDMFlows()
		require.NoError(c, err)

		destinations := tcpDestinationsFromNDMFlows(ndmflows)
		require.NotEmpty(c, destinations, "no TCP NetFlow destinations with valid IP and port after fakeintake reset")
	}, 2*time.Minute, 10*time.Second)

	require.Never(s.T(), func() bool {
		if path, err := firstNetflowNetworkPath(fakeintake); err != nil {
			s.T().Logf("error querying netpath events: %v", err)
			return true
		} else if path != nil {
			s.T().Logf("unexpected netflow network path event: destination=%s:%d test_run_id=%s",
				path.Destination.Hostname,
				path.Destination.Port,
				path.TestRunID,
			)
			return true
		}

		for _, metricName := range []string{
			"datadog.network_path.collector.schedule.pathtest_count",
			"datadog.network_path.collector.flush.pathtest_count",
		} {
			present, err := metricPresent(fakeintake, metricName)
			if err != nil {
				s.T().Logf("error querying metric %q: %v", metricName, err)
				return true
			}
			if present {
				s.T().Logf("unexpected network path collector metric %q", metricName)
				return true
			}
		}

		return false
	}, 1*time.Minute, 10*time.Second, "netflow dynamic path output was produced while network_path.netflow_monitoring.enabled was false")
}

func (s *netflowDynamicPathSuite) agentContainerIPv4() netip.Addr {
	output := s.Env().RemoteHost.MustExecute(`docker inspect -f '{{range .NetworkSettings.Networks}}{{println .IPAddress}}{{end}}' datadog-agent | awk 'NF {print; exit}'`)
	ip, err := netip.ParseAddr(strings.TrimSpace(output))
	require.NoError(s.T(), err, "could not parse Agent container IP from docker inspect output: %q", output)
	require.True(s.T(), ip.Is4(), "Agent container IP must be IPv4 for NetFlow v5: %s", ip)
	return ip
}

func (s *netflowDynamicPathSuite) sendSelfSourceNetflow(agentIP netip.Addr) {
	command := fmt.Sprintf(
		"python3 -c %s %s %s %d",
		shellQuote(sendSelfSourceNetflowScript),
		shellQuote(agentIP.String()),
		shellQuote(selfSourceNetflowDestinationIP),
		selfSourceNetflowDestinationPort,
	)
	s.Env().RemoteHost.MustExecute(command)
}

func (s *netflowDynamicPathSuite) stopNetflowGenerator() {
	s.Env().RemoteHost.MustExecute("docker stop netflow-generator || true")
}

func (s *netflowDynamicPathSuite) startNetflowGenerator() {
	if _, err := s.Env().RemoteHost.Execute("docker start netflow-generator || true"); err != nil {
		s.T().Logf("failed to restart netflow-generator: %v", err)
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
			ip:        ip.String(),
			port:      uint16(port),
			flowType:  flow.FlowType,
			namespace: flow.Device.Namespace,
		}
		destinations[destination.key()] = destination
	}
	return destinations
}

func findSelfSourceNDMFlow(flows []*aggregator.NDMFlow, sourceIP string, destinationIP string, destinationPort uint16) *aggregator.NDMFlow {
	for _, flow := range flows {
		if flow == nil || flow.IPProtocol != "TCP" || flow.FlowType != expectedNetflowFlowType {
			continue
		}
		if flow.Device.Namespace != expectedNetflowNamespace {
			continue
		}
		if flow.Source.IP == sourceIP &&
			flow.Destination.IP == destinationIP &&
			flow.Destination.Port == strconv.Itoa(int(destinationPort)) {
			return flow
		}
	}
	return nil
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

func findNetflowPathByDestination(netpaths []*aggregator.Netpath, destinationIP string, destinationPort uint16) *aggregator.Netpath {
	for _, np := range netpaths {
		if np == nil || np.Origin != payload.PathOriginNetflow || np.Protocol != payload.ProtocolTCP {
			continue
		}
		if np.Destination.Hostname == destinationIP && np.Destination.Port == destinationPort {
			return np
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

func firstNetflowNetworkPath(fakeintake *fakeintakeclient.Client) (*aggregator.Netpath, error) {
	netpaths, err := fakeintake.GetLatestNetpathEvents()
	if err != nil {
		return nil, err
	}
	for _, np := range netpaths {
		if np != nil && np.Origin == payload.PathOriginNetflow {
			return np, nil
		}
	}
	return nil, nil
}

func assertMetricPresent(c *assert.CollectT, fakeintake *fakeintakeclient.Client, metricName string) {
	present, err := metricPresent(fakeintake, metricName)
	require.NoError(c, err)
	assert.True(c, present, "metric %q not found", metricName)
}

func metricPresent(fakeintake *fakeintakeclient.Client, metricName string) (bool, error) {
	metrics, err := fakeintake.FilterMetrics(metricName)
	return len(metrics) > 0, err
}

func metricPresentWithTags(fakeintake *fakeintakeclient.Client, metricName string, tags []string) (bool, error) {
	metrics, err := fakeintake.FilterMetrics(metricName, fakeintakeclient.WithTags[*aggregator.MetricSeries](tags))
	return len(metrics) > 0, err
}

func (d netflowTCPDestination) key() string {
	return destinationKey(d.ip, d.port)
}

func destinationKey(ip string, port uint16) string {
	return fmt.Sprintf("%s:%d", ip, port)
}
