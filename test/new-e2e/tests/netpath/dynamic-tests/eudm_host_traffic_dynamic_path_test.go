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

//go:embed config/eudm_host_traffic_dynamic_path.yaml
var eudmHostTrafficDynamicPathAgentConfig string

const eudmHostTrafficSystemProbeConfig = `network_config:
  enabled: false
  direct_send: false
`

type eudmHostTrafficDynamicPathSuite struct {
	hostTrafficDynamicPathSuite
}

// TestEUDMHostTrafficDynamicPathSuite verifies default-on EUDM basic tests from packaged Agent configuration through fakeintake.
func TestEUDMHostTrafficDynamicPathSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &eudmHostTrafficDynamicPathSuite{}, e2e.WithProvisioner(hostTrafficDynamicPathProvisioner("eudmHostTrafficDynamicPath", eudmHostTrafficDynamicPathAgentConfig, eudmHostTrafficSystemProbeConfig)))
}

func (s *eudmHostTrafficDynamicPathSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	s.ensureCurlInstalled()
	s.startHostTrafficDNSServer()
	s.configureAgentResolver()
	s.assertHostTrafficDomainResolves()

	// Restart the process-agent after setup so the first observation window sees
	// the generated traffic. CNM is off; EUDM must start both network collection
	// and traceroute for this payload to arrive.
	s.Env().RemoteHost.MustExecute("sudo systemctl restart datadog-agent-process.service")
	require.NoError(s.T(), s.Env().FakeIntake.Client().FlushServerAndResetAggregators())
}

func (s *eudmHostTrafficDynamicPathSuite) TearDownSuite() {
	s.stopHostTrafficGenerator()
	s.restoreAgentResolver()
	s.stopHostTrafficDNSServer()
	s.BaseSuite.TearDownSuite()
}

func (s *eudmHostTrafficDynamicPathSuite) TestHostTrafficDynamicNetworkPath() {
	fakeintake := s.Env().FakeIntake.Client()
	s.startHostTrafficGenerator(6 * time.Minute)

	s.EventuallyWithT(func(c *assert.CollectT) {
		netpaths, err := fakeintake.GetLatestNetpathEvents()
		require.NoError(c, err)
		require.NotEmpty(c, netpaths, "no network path events")

		match := findHostTrafficNetworkPath(netpaths, hostTrafficRemoteConfigDomain)
		require.NotNil(c, match, "no EUDM basic path matched %s:80", hostTrafficRemoteConfigDomain)

		assert.Equal(c, payload.SourceProductEndUserDevice, match.SourceProduct)
		assert.Equal(c, payload.TestRunTypeDynamic, match.TestRunType)
		assert.Equal(c, payload.DynamicTestProfileBasic, match.DynamicTestProfile)
		assert.Equal(c, payload.DynamicTestClassCore, match.DynamicTestClass)
		assert.Equal(c, payload.CollectorTypeAgent, match.CollectorType)
		require.NotEmpty(c, match.Traceroute.Runs, "matched network path has no traceroute runs")
		assert.True(c, hasTracerouteDestinationIP(match), "matched network path has no traceroute destination IP")
	}, 7*time.Minute, 10*time.Second)
}
