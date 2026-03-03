// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package netpath contains e2e tests for Network Path Integration feature
package networkpathintegration

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
)

//go:embed fake-traceroute/datadog_ttl.yaml
var datadogYaml []byte

//go:embed fake-traceroute/network_path.yaml
var fakeNetworkPathYaml []byte

//go:embed fake-traceroute/router_setup.sh
var fakeRouterSetupScript []byte

//go:embed fake-traceroute/router_teardown.sh
var fakeRouterTeardownScript []byte

type fakeTracerouteTestSuite struct {
	baseNetworkPathIntegrationTestSuite
}

func TestFakeTracerouteSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &fakeTracerouteTestSuite{}, e2e.WithProvisioner(awshost.Provisioner(
		awshost.WithRunOptions(
			scenec2.WithAgentOptions(
				agentparams.WithAgentConfig(string(datadogYaml)),
				agentparams.WithSystemProbeConfig(string(sysProbeConfig)),
				agentparams.WithIntegration("network_path.d", string(fakeNetworkPathYaml)),
				agentparams.WithFile("/tmp/router_setup.sh", string(fakeRouterSetupScript), false),
				agentparams.WithFile("/tmp/router_teardown.sh", string(fakeRouterTeardownScript), false),
			)),
	),
	))

}

func (s *fakeTracerouteTestSuite) TestFakeTraceroute() {
	t := s.T()

	t.Cleanup(func() {
		s.Env().RemoteHost.MustExecute("sudo sh /tmp/router_teardown.sh")
	})
	s.Env().RemoteHost.MustExecute("sudo sh /tmp/router_setup.sh")

	routerIP := net.ParseIP("192.0.2.2")
	targetIP := net.ParseIP("198.51.100.2")

	hostname := s.Env().Agent.Client.Hostname()

	validatePath := func(c *assert.CollectT, np *aggregator.Netpath) {
		assert.Equal(c, payload.PathOrigin("network_path_integration"), np.Origin)
		assert.NotEmpty(c, np.TestRunID)
		assert.Equal(c, "default", np.Namespace)

		// check that the timestamp is reasonably close to the current time
		tolerance := time.Hour
		assert.Greater(c, np.Timestamp, time.Now().Add(-tolerance).UnixMilli())
		assert.Less(c, np.Timestamp, time.Now().Add(tolerance).UnixMilli())

		assert.Equal(c, hostname, np.Source.Hostname)
		assert.Equal(c, targetIP.String(), np.Destination.Hostname)

		require.Len(c, np.Traceroute.Runs, 3) // runs 3 traceroute by default

		// Validate all 3 traceroute runs
		for i, run := range np.Traceroute.Runs {
			assert.NotEmpty(c, run.RunID, "Run %d should have a RunID", i+1)
			assert.NotEmpty(c, run.Source.IPAddress, "Run %d should have source IP", i+1)
			assert.NotZero(c, run.Source.Port, "Run %d should have source port", i+1)
			assert.NotEmpty(c, run.Destination.IPAddress, "Run %d should have destination IP", i+1)
			assert.NotZero(c, run.Destination.Port, "Run %d should have destination port", i+1)

			require.Len(c, run.Hops, 2, "Run %d should have exactly 2 hops", i+1)

			// Validate first hop (router)
			assert.Equal(c, 1, run.Hops[0].TTL, "Run %d hop 1 should have TTL=1", i+1)
			assert.Equal(c, routerIP, run.Hops[0].IPAddress, "Run %d hop 1 should be router IP", i+1)
			assert.True(c, run.Hops[0].Reachable, "Run %d hop 1 should be reachable", i+1)

			// Validate second hop (target)
			assert.Equal(c, 2, run.Hops[1].TTL, "Run %d hop 2 should have TTL=2", i+1)
			assert.Equal(c, targetIP, run.Hops[1].IPAddress, "Run %d hop 2 should be target IP", i+1)
			assert.True(c, run.Hops[1].Reachable, "Run %d hop 2 should be reachable", i+1)
		}

		// Validate e2e probe statistics
		require.Len(c, np.E2eProbe.RTTs, 50) // runs 50 e2e probes by default
		assert.Equal(c, 50, np.E2eProbe.PacketsSent, "Should send exactly 50 packets")
		assert.Equal(c, 50, np.E2eProbe.PacketsReceived, "Should receive exactly 50 packets")
		assert.Equal(c, float32(0), np.E2eProbe.PacketLossPercentage, "Should have 0% packet loss")
	}

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		nps, err := s.Env().FakeIntake.Client().GetLatestNetpathEvents()
		assert.NoError(c, err, "GetLatestNetpathEvents() errors")
		if !assert.NotNil(c, nps, "GetLatestNetpathEvents() returned nil netpaths") {
			return
		}

		udpPath := s.expectNetpath(c, func(np *aggregator.Netpath) bool {
			return np.Destination.Hostname == targetIP.String() && np.Protocol == "UDP"
		})
		tcpPath := s.expectNetpath(c, func(np *aggregator.Netpath) bool {
			return np.Destination.Hostname == targetIP.String() && np.Protocol == "TCP"
		})

		if isNetpathDebugMode() {
			// Print payloads when debug mode, this helps debugging tests during development time
			tcpPathJSON, err := json.Marshal(tcpPath)
			assert.NoError(c, err)
			fmt.Println("TCP PATH: ", string(tcpPathJSON))
			udpPathJSON, err := json.Marshal(udpPath)
			assert.NoError(c, err)
			fmt.Println("UDP PATH: ", string(udpPathJSON))
		}

		validatePath(c, udpPath)
		validatePath(c, tcpPath)

		assert.Equal(c, uint16(0), udpPath.Destination.Port)
		assert.Equal(c, uint16(443), tcpPath.Destination.Port)

	}, 5*time.Minute, 3*time.Second)
}
