// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package networkpathdynamictests

import (
	_ "embed"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
)

//go:embed config/baseline_host_traffic_dynamic_path.yaml
var baselineHostTrafficDynamicPathAgentConfig string

type baselineHostTrafficDynamicPathSuite struct {
	hostTrafficDynamicPathSuite
}

// TestBaselineHostTrafficDynamicPathSuite verifies baseline tests from packaged Agent configuration through fakeintake.
func TestBaselineHostTrafficDynamicPathSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &baselineHostTrafficDynamicPathSuite{}, e2e.WithProvisioner(hostTrafficDynamicPathProvisioner("baselineHostTrafficDynamicPath", baselineHostTrafficDynamicPathAgentConfig)))
}

func (s *baselineHostTrafficDynamicPathSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	defer s.CleanupOnSetupFailure()
	s.startHostTrafficDNSServer()
	s.configureAgentResolver()
	s.assertHostTrafficDomainResolves()

	// Start the five-minute bootstrap window after infrastructure setup so the
	// test traffic cannot miss it and fall into the next ten-minute window.
	require.NoError(s.T(), s.Env().Agent.Client.Restart())
	s.EventuallyWithT(func(c *assert.CollectT) {
		assert.True(c, s.Env().Agent.Client.IsReady(), "Agent did not become ready after restart")
	}, time.Minute, 5*time.Second)
	require.NoError(s.T(), s.Env().FakeIntake.Client().FlushServerAndResetAggregators())
}

func (s *baselineHostTrafficDynamicPathSuite) TearDownSuite() {
	s.stopHostTrafficGenerator()
	s.restoreAgentResolver()
	s.stopHostTrafficDNSServer()
	s.BaseSuite.TearDownSuite()
}

func (s *baselineHostTrafficDynamicPathSuite) TestHostTrafficDynamicNetworkPath() {
	fakeintake := s.Env().FakeIntake.Client()
	s.startHostTrafficGenerator(6 * time.Minute)

	s.EventuallyWithT(func(c *assert.CollectT) {
		netpaths, err := fakeintake.GetLatestNetpathEvents()
		require.NoError(c, err)
		require.NotEmpty(c, netpaths, "no network path events")

		match := findHostTrafficNetworkPath(netpaths, hostTrafficRemoteConfigDomain)
		require.NotNil(c, match, "no baseline host-traffic network path event matched %s:80", hostTrafficRemoteConfigDomain)

		assert.Equal(c, payload.PathOriginNetworkTraffic, match.Origin)
		assert.Equal(c, payload.SourceProductNetworkPath, match.SourceProduct)
		assert.Equal(c, payload.TestRunTypeDynamic, match.TestRunType)
		assert.Equal(c, payload.DynamicTestProfileBaseline, match.DynamicTestProfile)
		assert.Equal(c, payload.CollectorTypeAgent, match.CollectorType)
		assert.Equal(c, payload.ProtocolTCP, match.Protocol)
		assert.Equal(c, hostTrafficRemoteConfigDomain, match.Destination.Hostname)
		assert.Equal(c, uint16(80), match.Destination.Port)
		require.NotEmpty(c, match.Traceroute.Runs, "matched network path has no traceroute runs")
		assert.True(c, hasTracerouteDestinationIP(match), "matched network path has no traceroute destination IP")
	}, 7*time.Minute, 10*time.Second)
}
