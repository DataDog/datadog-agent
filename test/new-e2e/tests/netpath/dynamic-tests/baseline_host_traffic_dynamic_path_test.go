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

//go:embed config/baseline_host_traffic_system_probe.yaml
var baselineHostTrafficSystemProbeConfig string

type baselineHostTrafficDynamicPathSuite struct {
	hostTrafficDynamicPathBaseSuite
}

// TestBaselineHostTrafficDynamicPathSuite verifies baseline tests from packaged Agent configuration through fakeintake.
func TestBaselineHostTrafficDynamicPathSuite(t *testing.T) {
	e2e.Run(t, &baselineHostTrafficDynamicPathSuite{}, e2e.WithProvisioner(hostTrafficDynamicPathProvisioner("baselineHostTrafficDynamicPath", baselineHostTrafficDynamicPathAgentConfig, baselineHostTrafficSystemProbeConfig)))
}

func (s *baselineHostTrafficDynamicPathSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	defer s.CleanupOnSetupFailure()
	s.setupHostTraffic()
	require.NoError(s.T(), s.Env().FakeIntake.Client().FlushServerAndResetAggregators())
}

func (s *baselineHostTrafficDynamicPathSuite) TearDownSuite() {
	s.tearDownHostTraffic()
	s.BaseSuite.TearDownSuite()
}

func (s *baselineHostTrafficDynamicPathSuite) TestHostTrafficDynamicNetworkPath() {
	fakeintake := s.Env().FakeIntake.Client()
	s.startHostTrafficGenerator(4 * time.Minute)

	s.EventuallyWithT(func(c *assert.CollectT) {
		assertMetricPresent(c, fakeintake, "datadog.network_path.collector.baseline.selections")
		assertMetricPresent(c, fakeintake, "datadog.network_path.store.baseline_dispatched")

		netpaths, err := fakeintake.GetLatestNetpathEvents()
		require.NoError(c, err)
		require.NotEmpty(c, netpaths, "no network path events")

		assertHostTrafficNetworkPath(c, netpaths, payload.DynamicTestProfileBaseline, "baseline")
	}, 5*time.Minute, 10*time.Second)
}
