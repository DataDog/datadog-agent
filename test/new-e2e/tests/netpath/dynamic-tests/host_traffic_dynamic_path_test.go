// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package networkpathdynamictests

import (
	_ "embed"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
)

//go:embed config/host_traffic_dynamic_path.yaml
var hostTrafficDynamicPathAgentConfig string

//go:embed config/host_traffic_system_probe.yaml
var hostTrafficSystemProbeConfig string

const (
	hostTrafficRCProduct    = "NETWORK_PATH"
	hostTrafficRCConfigID   = "test-config-dynamic-host-traffic"
	hostTrafficRCConfigName = "config"
)

var hostTrafficDynamicRCConfig = []byte(`{
  "type": "dynamic",
  "test_config_id": "dynamic-host-traffic",
  "test_config_name": "Host traffic dynamic paths",
  "tags": ["team:netpath", "env:e2e"],
  "config": {
    "filters": [
      {
        "type": "include",
        "match_domain": "httpbin-rc.dynamic-netpath.test",
        "match_domain_strategy": "wildcard"
      }
    ]
  }
}`)

type hostTrafficDynamicPathSuite struct {
	hostTrafficDynamicPathBaseSuite
	remoteConfigAdded bool
}

// TestHostTrafficDynamicPathSuite runs Network Path Dynamic Tests backed by host NPM traffic.
func TestHostTrafficDynamicPathSuite(t *testing.T) {
	e2e.Run(t, &hostTrafficDynamicPathSuite{}, e2e.WithProvisioner(hostTrafficDynamicPathProvisioner("hostTrafficDynamicPath", hostTrafficDynamicPathAgentConfig, hostTrafficSystemProbeConfig)))
}

func (s *hostTrafficDynamicPathSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	defer s.CleanupOnSetupFailure()
	s.setupHostTraffic()

	fakeintake := s.Env().FakeIntake.Client()
	s.EventuallyWithT(func(c *assert.CollectT) {
		stats, err := fakeintake.RCStats()
		assert.NoError(c, err)
		assert.NotZero(c, stats.Polls, "agent did not poll fakeintake Remote Config")
	}, 2*time.Minute, 5*time.Second)
	require.NoError(s.T(), fakeintake.RCAddConfig("", hostTrafficRCProduct, hostTrafficRCConfigID, hostTrafficRCConfigName, hostTrafficDynamicRCConfig))
	s.remoteConfigAdded = true
	statsAfterAdd, err := fakeintake.RCStats()
	require.NoError(s.T(), err)
	s.EventuallyWithT(func(c *assert.CollectT) {
		stats, err := fakeintake.RCStats()
		assert.NoError(c, err)
		assert.Greater(c, stats.Polls, statsAfterAdd.Polls, "agent did not poll Remote Config after the dynamic config was added")
	}, 2*time.Minute, 5*time.Second)

	require.NoError(s.T(), fakeintake.FlushServerAndResetAggregators())
}

func (s *hostTrafficDynamicPathSuite) TearDownSuite() {
	s.deleteHostTrafficRemoteConfig()
	s.tearDownHostTraffic()
	s.BaseSuite.TearDownSuite()
}

func (s *hostTrafficDynamicPathSuite) TestHostTrafficDynamicNetworkPath() {
	fakeintake := s.Env().FakeIntake.Client()
	s.startHostTrafficGenerator(4 * time.Minute)

	var remoteConfigMatch *aggregator.Netpath
	s.EventuallyWithT(func(c *assert.CollectT) {
		assertMetricPresent(c, fakeintake, "datadog.network_path.collector.schedule.pathtest_count")
		assertMetricPresent(c, fakeintake, "datadog.network_path.collector.flush.pathtest_count")

		netpaths, err := fakeintake.GetLatestNetpathEvents()
		require.NoError(c, err)
		require.NotEmpty(c, netpaths, "no network path events")

		match := assertHostTrafficNetworkPath(c, netpaths, "", "RC-admitted")
		assert.Equal(c, "dynamic-host-traffic", match.TestConfigID)
		assert.Equal(c, "Host traffic dynamic paths", match.TestConfigName)
		assert.Equal(c, payload.TestConfigSourceRemote, match.TestConfigSource)
		assert.Equal(c, []string{"team:netpath", "env:e2e"}, match.Tags)
		remoteConfigMatch = match
	}, 5*time.Minute, 10*time.Second)

	if remoteConfigMatch != nil {
		s.T().Logf("matched RC host traffic dynamic path destination=%s:%d test_run_id=%s",
			remoteConfigMatch.Destination.Hostname,
			remoteConfigMatch.Destination.Port,
			remoteConfigMatch.TestRunID,
		)
	}
}

func (s *hostTrafficDynamicPathSuite) deleteHostTrafficRemoteConfig() {
	if !s.remoteConfigAdded {
		return
	}
	fakeintake := s.Env().FakeIntake.Client()
	configs, err := fakeintake.RCListConfigs()
	require.NoError(s.T(), err)
	for _, config := range configs {
		if config.Product != hostTrafficRCProduct || config.ConfigID != hostTrafficRCConfigID || config.ConfigName != hostTrafficRCConfigName {
			continue
		}
		key := fmt.Sprintf("%s/%s/%s/%s", config.OrgID, config.Product, config.ConfigID, config.ConfigName)
		require.NoError(s.T(), fakeintake.RCDeleteConfig(key))
		s.remoteConfigAdded = false
		return
	}
	require.Failf(s.T(), "Remote Config entry not found", "product=%s config_id=%s config_name=%s", hostTrafficRCProduct, hostTrafficRCConfigID, hostTrafficRCConfigName)
}
