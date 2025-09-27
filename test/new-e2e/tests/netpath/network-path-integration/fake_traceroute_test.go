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

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
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
		awshost.WithAgentOptions(
			agentparams.WithAgentConfig(string(datadogYaml)),
			agentparams.WithSystemProbeConfig(string(sysProbeConfig)),
			agentparams.WithIntegration("network_path.d", string(fakeNetworkPathYaml)),
			agentparams.WithFile("/tmp/router_setup.sh", string(fakeRouterSetupScript), false),
			agentparams.WithFile("/tmp/router_teardown.sh", string(fakeRouterTeardownScript), false),
		)),
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

	validateCommonPath := func(c *assert.CollectT, np *aggregator.Netpath) {
		assert.Equal(c, payload.PathOrigin("network_path_integration"), np.Origin)
		assert.NotEmpty(c, np.PathtraceID)
		assert.Equal(c, "default", np.Namespace)

		// check that the timestamp is reasonably close to the current time
		tolerance := time.Hour
		assert.Greater(c, np.Timestamp, time.Now().Add(-tolerance).UnixMilli())
		assert.Less(c, np.Timestamp, time.Now().Add(tolerance).UnixMilli())

		assert.Equal(c, hostname, np.Source.Hostname)
		assert.Equal(c, targetIP.String(), np.Destination.Hostname)

		// Validate e2e probe statistics
		require.Len(c, np.E2eProbe.RTTs, 50) // runs 50 e2e probes by default
		assert.Equal(c, 50, np.E2eProbe.PacketsSent, "Should send exactly 50 packets")
		assert.Equal(c, 50, np.E2eProbe.PacketsReceived, "Should receive exactly 50 packets")
		assert.Equal(c, float32(0), np.E2eProbe.PacketLossPercentage, "Should have 0% packet loss")
	}

	validateTCPTraceroute := func(c *assert.CollectT, np *aggregator.Netpath) {
		validateCommonPath(c, np)

		// TCP traceroutes should have exactly 3 runs
		require.Len(c, np.Traceroute.Runs, 3, "TCP traceroute should have exactly 3 runs")

		// Check that at least one run has the router IP as the first hop
		foundRouterAsFirstHop := false

		// Validate all 3 traceroute runs
		for i, run := range np.Traceroute.Runs {
			assert.NotEmpty(c, run.RunID, "TCP run %d should have a RunID", i+1)
			assert.NotEmpty(c, run.Source.IPAddress, "TCP run %d should have source IP", i+1)
			assert.NotZero(c, run.Source.Port, "TCP run %d should have source port", i+1)
			assert.NotEmpty(c, run.Destination.IPAddress, "TCP run %d should have destination IP", i+1)
			assert.NotZero(c, run.Destination.Port, "TCP run %d should have destination port", i+1)

			// Each run should have exactly 2 hops
			require.Len(c, run.Hops, 2, "TCP run %d should have exactly 2 hops", i+1)

			// Validate first hop (router) - may be unreachable for TCP
			assert.Equal(c, 1, run.Hops[0].TTL, "TCP run %d hop 1 should have TTL=1", i+1)
			if run.Hops[0].IPAddress.Equal(routerIP) {
				foundRouterAsFirstHop = true
			}

			// Validate second hop (target) - destination must be reachable
			assert.Equal(c, 2, run.Hops[1].TTL, "TCP run %d hop 2 should have TTL=2", i+1)
			assert.Equal(c, targetIP, run.Hops[1].IPAddress, "TCP run %d hop 2 should be target IP", i+1)
			assert.True(c, run.Hops[1].Reachable, "TCP run %d destination hop should be reachable", i+1)
		}

		// Ensure at least one run has the router as the first hop
		assert.True(c, foundRouterAsFirstHop, "At least one TCP run should have the router IP as the first hop")
	}

	validateUDPTraceroute := func(c *assert.CollectT, np *aggregator.Netpath) {
		validateCommonPath(c, np)

		// UDP traceroutes should have exactly 3 runs
		require.Len(c, np.Traceroute.Runs, 3, "UDP traceroute should have exactly 3 runs")

		// Check that at least one run reaches the destination with reachable status
		foundReachableDestination := false
		for i, run := range np.Traceroute.Runs {
			assert.NotEmpty(c, run.RunID, "UDP run %d should have a RunID", i+1)
			require.NotEmpty(c, run.Hops, "UDP run %d should have at least one hop", i+1)

			// Check if this run has a reachable destination hop
			for _, hop := range run.Hops {
				if hop.IPAddress.Equal(targetIP) && hop.Reachable {
					foundReachableDestination = true
					break
				}
			}
		}

		assert.True(c, foundReachableDestination, "At least one UDP run should reach the destination with reachable status")
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

		validateUDPTraceroute(c, udpPath)
		validateTCPTraceroute(c, tcpPath)

		assert.Equal(c, uint16(0), udpPath.Destination.Port)
		assert.Equal(c, uint16(443), tcpPath.Destination.Port)

	}, 5*time.Minute, 3*time.Second)
}
