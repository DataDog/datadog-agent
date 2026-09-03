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

//go:embed config/basic_host_traffic_dynamic_path.yaml
var basicHostTrafficDynamicPathAgentConfig string

type basicHostTrafficDynamicPathSuite struct {
	hostTrafficDynamicPathSuite
}

// TestBasicHostTrafficDynamicPathSuite verifies basic tests from packaged Agent configuration through fakeintake.
func TestBasicHostTrafficDynamicPathSuite(t *testing.T) {
	e2e.Run(t, &basicHostTrafficDynamicPathSuite{}, e2e.WithProvisioner(hostTrafficDynamicPathProvisioner("basicHostTrafficDynamicPath", basicHostTrafficDynamicPathAgentConfig)))
}

func (s *basicHostTrafficDynamicPathSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	s.ensureCurlInstalled()
	s.startHostTrafficDNSServer()
	s.configureAgentResolver()
	s.assertHostTrafficDomainResolves()

	// direct_send is false, so process-agent owns the selector. Restart it after
	// infrastructure setup so traffic cannot miss the five-minute bootstrap window.
	s.Env().RemoteHost.MustExecute("sudo systemctl restart datadog-agent-process.service")
	require.NoError(s.T(), s.Env().FakeIntake.Client().FlushServerAndResetAggregators())
}

func (s *basicHostTrafficDynamicPathSuite) TearDownSuite() {
	s.stopHostTrafficGenerator()
	s.restoreAgentResolver()
	s.stopHostTrafficDNSServer()
	s.BaseSuite.TearDownSuite()
}

func (s *basicHostTrafficDynamicPathSuite) TestHostTrafficDynamicNetworkPath() {
	fakeintake := s.Env().FakeIntake.Client()
	s.startHostTrafficGenerator(6 * time.Minute)

	s.EventuallyWithT(func(c *assert.CollectT) {
		netpaths, err := fakeintake.GetLatestNetpathEvents()
		require.NoError(c, err)
		require.NotEmpty(c, netpaths, "no network path events")

		match := findHostTrafficNetworkPath(netpaths, hostTrafficRemoteConfigDomain)
		require.NotNil(c, match, "no basic host-traffic network path event matched %s:80", hostTrafficRemoteConfigDomain)

		assert.Equal(c, payload.SourceProductNetworkPath, match.SourceProduct)
		assert.Equal(c, payload.TestRunTypeDynamic, match.TestRunType)
		assert.Equal(c, payload.DynamicTestProfileBasic, match.DynamicTestProfile)
		assert.Equal(c, payload.CollectorTypeAgent, match.CollectorType)
		require.NotEmpty(c, match.Traceroute.Runs, "matched network path has no traceroute runs")
		assert.True(c, hasTracerouteDestinationIP(match), "matched network path has no traceroute destination IP")
	}, 7*time.Minute, 10*time.Second)
}
