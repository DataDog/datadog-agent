// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package networkpathintegration contains e2e tests for the Network Path Integration feature.
package networkpathintegration

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"
)

//go:embed fake-traceroute/rc_local.yaml
var remoteConfigLocalNetworkPathYaml []byte

var remoteConfigAgentDatadogYaml = []byte(`
network_path:
  remote_config:
    enabled: true
  collector:
    max_ttl: 10
`)

const (
	networkPathRCProduct         = "NETWORK_PATH"
	networkPathRCConfigID        = "test-config-aaa-bbb-ccc"
	networkPathRCDynamicConfigID = "test-config-dynamic-sentinel"
	networkPathRCConfigName      = "config"
	networkPathTestConfigID      = "aaa-bbb-ccc"
)

var linuxScheduledNetworkPathRCConfig = []byte(`{
  "type": "scheduled",
  "test_config_id": "aaa-bbb-ccc",
  "config": {
    "tests": [
      {
        "hostname": "198.51.100.2",
        "protocol": "UDP",
        "interval_sec": 10,
        "max_ttl": 10,
        "traceroute_queries": 1,
        "e2e_queries": 1
      },
      {
        "hostname": "198.51.100.2",
        "protocol": "TCP",
        "port": 443,
        "interval_sec": 10,
        "max_ttl": 10,
        "traceroute_queries": 1,
        "e2e_queries": 1
      }
    ]
  }
}`)

var crossPlatformScheduledNetworkPathRCConfig = []byte(`{
  "type": "scheduled",
  "test_config_id": "aaa-bbb-ccc",
  "config": {
    "tests": [
      {
        "hostname": "api.datadoghq.eu",
        "protocol": "TCP",
        "port": 443,
        "interval_sec": 10,
        "max_ttl": 10,
        "traceroute_queries": 1,
        "e2e_queries": 1
      }
    ]
  }
}`)

var dynamicNetworkPathRCConfig = []byte(`{
  "type": "dynamic",
  "test_config_id": "dynamic-sentinel",
  "config": {
    "filters": [
      {
        "type": "exclude",
        "match_domain": "*.ignored.example.com",
        "match_domain_strategy": "wildcard"
      }
    ]
  }
}`)

type remoteConfigTestSuite struct {
	baseNetworkPathIntegrationTestSuite
	platform         remoteConfigTestPlatform
	scheduledConfig  []byte
	expectedPaths    []remoteConfigPathExpectation
	localConfigCount int
}

type remoteConfigTestPlatform string

const (
	remoteConfigPlatformLinux   remoteConfigTestPlatform = "linux"
	remoteConfigPlatformWindows remoteConfigTestPlatform = "windows"
)

type remoteConfigPathExpectation struct {
	hostname         string
	protocol         payload.Protocol
	port             uint16
	configSubstrings []string
}

type configCheckEntry struct {
	CheckName string `json:"check_name"`
	Source    string `json:"source"`
	Instances []struct {
		ID     string `json:"id"`
		Config string `json:"config"`
	} `json:"instances"`
}

func remoteConfigAgentOptions() []agentparams.Option {
	return []agentparams.Option{
		agentparams.WithAgentConfig(string(remoteConfigAgentDatadogYaml)),
		agentparams.WithSystemProbeConfig(string(sysProbeConfig)),
	}
}

func (s *remoteConfigTestSuite) TestScheduledNetworkPathRemoteConfig() {
	t := s.T()
	if s.Env().Agent.FIPSEnabled {
		t.Skip("Remote Config is not supported by the FIPS Agent")
	}

	s.preparePlatform()

	agentHostname := s.Env().Agent.Client.Hostname()
	fakeIntakeClient := s.Env().FakeIntake.Client()

	s.EventuallyWithT(func(c *assert.CollectT) {
		assert.True(c, s.Env().Agent.Client.IsReady())
	}, 2*time.Minute, 5*time.Second)

	s.EventuallyWithT(func(c *assert.CollectT) {
		stats, err := fakeIntakeClient.RCStats()
		assert.NoError(c, err)
		assert.NotZero(c, stats.Polls, "agent did not poll fakeintake Remote Config")
	}, 2*time.Minute, 5*time.Second)

	// Keep one ignored NETWORK_PATH config so fakeintake returns a non-empty
	// snapshot after the scheduled config is deleted.
	require.NoError(t, fakeIntakeClient.RCAddConfig("", networkPathRCProduct, networkPathRCDynamicConfigID, networkPathRCConfigName, dynamicNetworkPathRCConfig))

	s.EventuallyWithT(func(c *assert.CollectT) {
		entries := s.networkPathConfigCheckEntries(c)
		assertNetworkPathConfigCount(c, entries, isLocalNetworkPathSource, s.localConfigCount)
		assertNetworkPathConfigCount(c, entries, isRemoteConfigNetworkPathSource, 0)
	}, 2*time.Minute, 5*time.Second)

	require.NoError(t, fakeIntakeClient.RCAddConfig("", networkPathRCProduct, networkPathRCConfigID, networkPathRCConfigName, s.scheduledConfig))

	s.EventuallyWithT(func(c *assert.CollectT) {
		entries := s.networkPathConfigCheckEntries(c)
		assertNetworkPathConfigCount(c, entries, isLocalNetworkPathSource, s.localConfigCount)
		assertNetworkPathConfigCount(c, entries, isRemoteConfigNetworkPathSource, len(s.expectedPaths))
		for _, expectedPath := range s.expectedPaths {
			assertNetworkPathConfig(c, entries, isRemoteConfigNetworkPathSource, expectedPath.configSubstrings...)
		}
	}, 3*time.Minute, 5*time.Second)

	s.EventuallyWithT(func(c *assert.CollectT) {
		if s.platform == remoteConfigPlatformLinux {
			s.assertLinuxLocalPaths(c, agentHostname)
		}
		for _, expectedPath := range s.expectedPaths {
			np := s.expectNetpath(c, agentHostname, func(np *aggregator.Netpath) bool {
				return np.Destination.Hostname == expectedPath.hostname && np.Protocol == expectedPath.protocol && np.Destination.Port == expectedPath.port && np.TestConfigID == networkPathTestConfigID
			})
			assertRemoteConfigPath(c, np, agentHostname, expectedPath)
			if s.platform == remoteConfigPlatformLinux {
				assertFakeTracerouteTopology(c, np, net.ParseIP("192.0.2.2"), net.ParseIP("198.51.100.2"))
			}
		}
	}, 5*time.Minute, 3*time.Second)

	s.deleteRemoteConfig(t, fakeIntakeClient)

	s.EventuallyWithT(func(c *assert.CollectT) {
		entries := s.networkPathConfigCheckEntries(c)
		assertNetworkPathConfigCount(c, entries, isLocalNetworkPathSource, s.localConfigCount)
		assertNetworkPathConfigCount(c, entries, isRemoteConfigNetworkPathSource, 0)
	}, 3*time.Minute, 5*time.Second)
}

func (s *remoteConfigTestSuite) assertLinuxLocalPaths(c *assert.CollectT, agentHostname string) {
	targetIP := net.ParseIP("198.51.100.2")
	routerIP := net.ParseIP("192.0.2.2")
	for _, port := range []uint16{80, 8080} {
		np := s.expectNetpath(c, agentHostname, func(np *aggregator.Netpath) bool {
			return np.Destination.Hostname == targetIP.String() && np.Protocol == payload.ProtocolTCP && np.Destination.Port == port && np.TestConfigID == ""
		})
		assertPayloadBase(c, np, agentHostname)
		assert.Equal(c, payload.SourceProductNetworkPath, np.SourceProduct)
		assert.Equal(c, targetIP.String(), np.Destination.Hostname)
		assert.Equal(c, port, np.Destination.Port)
		require.Len(c, np.Traceroute.Runs, 1)
		require.Len(c, np.E2eProbe.RTTs, 1)
		assertFakeTracerouteTopology(c, np, routerIP, targetIP)
	}
}

func (s *remoteConfigTestSuite) preparePlatform() {
	switch s.platform {
	case remoteConfigPlatformLinux:
		s.T().Cleanup(func() {
			s.Env().RemoteHost.MustExecute("sudo sh /tmp/router_teardown.sh")
		})
		s.Env().RemoteHost.MustExecute("sudo sh /tmp/router_setup.sh")
	case remoteConfigPlatformWindows:
		_, err := s.Env().RemoteHost.Host.Execute("Set-NetFirewallProfile -Profile Domain,Public,Private -Enabled False")
		s.Require().NoError(err)
	default:
		s.Require().Failf("unsupported platform", "platform=%q", s.platform)
	}
}

func (s *remoteConfigTestSuite) networkPathConfigCheckEntries(c *assert.CollectT) []configCheckEntry {
	output := s.Env().Agent.Client.ConfigCheck(agentclient.WithArgs([]string{"--json"}))

	var entries []configCheckEntry
	require.NoError(c, json.Unmarshal([]byte(output), &entries), "configcheck --json output: %s", output)

	var networkPathEntries []configCheckEntry
	for _, entry := range entries {
		if entry.CheckName == "network_path" {
			networkPathEntries = append(networkPathEntries, entry)
		}
	}
	return networkPathEntries
}

func isLocalNetworkPathSource(source string) bool {
	return strings.Contains(source, "network_path.d/conf.yaml")
}

func isRemoteConfigNetworkPathSource(source string) bool {
	return source == "network-path-remote-config:scheduled"
}

func assertNetworkPathConfigCount(c *assert.CollectT, entries []configCheckEntry, sourceMatches func(string) bool, expected int) {
	actual := 0
	for _, entry := range entries {
		if sourceMatches(entry.Source) {
			actual += len(entry.Instances)
		}
	}
	assert.Equal(c, expected, actual, "network_path configs: %+v", entries)
}

func assertNetworkPathConfig(c *assert.CollectT, entries []configCheckEntry, sourceMatches func(string) bool, expectedSubstrings ...string) {
	for _, entry := range entries {
		if !sourceMatches(entry.Source) {
			continue
		}
		for _, instance := range entry.Instances {
			if containsAll(instance.Config, expectedSubstrings...) {
				return
			}
		}
	}
	assert.Failf(c, "network_path config not found", "expected substrings %q in configs %+v", expectedSubstrings, entries)
}

func containsAll(value string, substrings ...string) bool {
	for _, substring := range substrings {
		if !strings.Contains(value, substring) {
			return false
		}
	}
	return true
}

func assertRemoteConfigPath(c *assert.CollectT, np *aggregator.Netpath, agentHostname string, expected remoteConfigPathExpectation) {
	assertPayloadBase(c, np, agentHostname)
	assert.Equal(c, payload.SourceProductNetworkPath, np.SourceProduct)
	assert.Equal(c, expected.hostname, np.Destination.Hostname)
	assert.Equal(c, expected.protocol, np.Protocol)
	assert.Equal(c, expected.port, np.Destination.Port)
	assert.Equal(c, networkPathTestConfigID, np.TestConfigID)

	require.Len(c, np.Traceroute.Runs, 1)
	run := np.Traceroute.Runs[0]
	assert.NotEmpty(c, run.RunID)
	assert.NotEmpty(c, run.Source.IPAddress)
	assert.NotZero(c, run.Source.Port)
	assert.NotEmpty(c, run.Destination.IPAddress)
	assert.NotEmpty(c, run.Hops)

	require.Len(c, np.E2eProbe.RTTs, 1)
	assert.Equal(c, 1, np.E2eProbe.PacketsSent)
}

func assertFakeTracerouteTopology(c *assert.CollectT, np *aggregator.Netpath, routerIP, targetIP net.IP) {
	run := np.Traceroute.Runs[0]
	assert.Equal(c, targetIP, run.Destination.IPAddress)
	require.Len(c, run.Hops, 2)
	assert.Equal(c, 1, run.Hops[0].TTL)
	assert.Equal(c, routerIP, run.Hops[0].IPAddress)
	assert.True(c, run.Hops[0].Reachable)
	assert.Equal(c, 2, run.Hops[1].TTL)
	assert.Equal(c, targetIP, run.Hops[1].IPAddress)
	assert.True(c, run.Hops[1].Reachable)

	assert.Equal(c, 1, np.E2eProbe.PacketsReceived)
	assert.Equal(c, float32(0), np.E2eProbe.PacketLossPercentage)
}

func (s *remoteConfigTestSuite) deleteRemoteConfig(t require.TestingT, fakeIntakeClient *fakeintakeclient.Client) {
	configs, err := fakeIntakeClient.RCListConfigs()
	require.NoError(t, err)
	for _, config := range configs {
		if config.Product == networkPathRCProduct && config.ConfigID == networkPathRCConfigID && config.ConfigName == networkPathRCConfigName {
			key := fmt.Sprintf("%s/%s/%s/%s", config.OrgID, config.Product, config.ConfigID, config.ConfigName)
			require.NoError(t, fakeIntakeClient.RCDeleteConfig(key))
			return
		}
	}
	require.Failf(t, "Remote Config entry not found", "product=%s config_id=%s config_name=%s", networkPathRCProduct, networkPathRCConfigID, networkPathRCConfigName)
}
