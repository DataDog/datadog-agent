// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package netpath contains e2e tests for Network Path Integration feature
package networkpathintegration

import (
	_ "embed"
	"testing"
	"time"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
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
	// TODO remove this if the PR fixes the flakiness
	flake.Mark(t)

	t.Cleanup(func() {
		s.Env().RemoteHost.MustExecute("sudo sh /tmp/router_teardown.sh")
	})
	s.Env().RemoteHost.MustExecute("sudo sh /tmp/router_setup.sh")

	routerIP := "192.0.2.2"
	targetIP := "198.51.100.2"

	hostname := s.Env().Agent.Client.Hostname()

	validatePath := func(c *assert.CollectT, np *aggregator.Netpath) {
		assert.Equal(c, payload.PathOrigin("network_path_integration"), np.Origin)
		assert.NotEmpty(c, np.PathtraceID)
		assert.Equal(c, "default", np.Namespace)

		// check that the timestamp is reasonably close to the current time
		tolerance := time.Hour
		assert.Greater(c, np.Timestamp, time.Now().Add(-tolerance).UnixMilli())
		assert.Less(c, np.Timestamp, time.Now().Add(tolerance).UnixMilli())

		assert.Equal(c, hostname, np.Source.Hostname)
		assert.Equal(c, targetIP, np.Destination.Hostname)
		assert.Equal(c, targetIP, np.Destination.IPAddress)
		assert.NotZero(c, np.Destination.Port)

		if !assert.Len(c, np.Hops, 2) {
			return
		}

		assert.Equal(c, 1, np.Hops[0].TTL)
		assert.Equal(c, routerIP, np.Hops[0].IPAddress)
		assert.Equal(c, routerIP, np.Hops[0].Hostname)
		assert.True(c, np.Hops[0].Reachable)

		assert.Equal(c, 2, np.Hops[1].TTL)
		assert.Equal(c, targetIP, np.Hops[1].IPAddress)
		assert.Equal(c, targetIP, np.Hops[1].Hostname)
		assert.True(c, np.Hops[1].Reachable)
	}

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		nps, err := s.Env().FakeIntake.Client().GetLatestNetpathEvents()
		assert.NoError(c, err, "GetLatestNetpathEvents() errors")
		if !assert.NotNil(c, nps, "GetLatestNetpathEvents() returned nil netpaths") {
			return
		}

		udpPath := s.expectNetpath(c, func(np *aggregator.Netpath) bool {
			return np.Destination.Hostname == targetIP && np.Protocol == "UDP"
		})
		tcpPath := s.expectNetpath(c, func(np *aggregator.Netpath) bool {
			return np.Destination.Hostname == targetIP && np.Protocol == "TCP"
		})

		validatePath(c, udpPath)
		validatePath(c, tcpPath)
		assert.Equal(c, uint16(443), tcpPath.Destination.Port)
	}, 5*time.Minute, 3*time.Second)
}
