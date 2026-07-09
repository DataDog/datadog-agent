// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package netpath contains e2e tests for Network Path Integration feature
package networkpathintegration

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
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

var scheduledNetworkPathRCConfig = []byte(`{
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
}

type configCheckEntry struct {
	CheckName string `json:"check_name"`
	Source    string `json:"source"`
	Instances []struct {
		ID     string `json:"id"`
		Config string `json:"config"`
	} `json:"instances"`
}

func TestRemoteConfigSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &remoteConfigTestSuite{}, e2e.WithProvisioner(awshost.Provisioner(
		awshost.WithRunOptions(
			scenec2.WithAgentOptions(
				agentparams.WithAgentConfig(string(remoteConfigAgentDatadogYaml)),
				agentparams.WithSystemProbeConfig(string(sysProbeConfig)),
				agentparams.WithIntegration("network_path.d", string(remoteConfigLocalNetworkPathYaml)),
				agentparams.WithFile("/tmp/router_setup.sh", string(fakeRouterSetupScript), false),
				agentparams.WithFile("/tmp/router_teardown.sh", string(fakeRouterTeardownScript), false),
			)),
	),
	))
}

func (s *remoteConfigTestSuite) TestScheduledNetworkPathRemoteConfig() {
	t := s.T()

	t.Cleanup(func() {
		s.Env().RemoteHost.MustExecute("sudo sh /tmp/router_teardown.sh")
	})
	s.Env().RemoteHost.MustExecute("sudo sh /tmp/router_setup.sh")

	agentHostname := s.Env().Agent.Client.Hostname()
	targetIP := net.ParseIP("198.51.100.2")
	routerIP := net.ParseIP("192.0.2.2")
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
		assertNetworkPathConfigCount(c, entries, isLocalNetworkPathSource, 2)
		assertNetworkPathConfigCount(c, entries, isRemoteConfigNetworkPathSource, 0)
	}, 2*time.Minute, 5*time.Second)

	require.NoError(t, fakeIntakeClient.RCAddConfig("", networkPathRCProduct, networkPathRCConfigID, networkPathRCConfigName, scheduledNetworkPathRCConfig))

	s.EventuallyWithT(func(c *assert.CollectT) {
		entries := s.networkPathConfigCheckEntries(c)
		assertNetworkPathConfigCount(c, entries, isLocalNetworkPathSource, 2)
		assertNetworkPathConfigCount(c, entries, isRemoteConfigNetworkPathSource, 2)
		assertNetworkPathConfig(c, entries, isRemoteConfigNetworkPathSource, "hostname: 198.51.100.2", "protocol: UDP", "test_config_id: aaa-bbb-ccc")
		assertNetworkPathConfig(c, entries, isRemoteConfigNetworkPathSource, "hostname: 198.51.100.2", "protocol: TCP", "port: 443", "test_config_id: aaa-bbb-ccc")
	}, 3*time.Minute, 5*time.Second)

	s.EventuallyWithT(func(c *assert.CollectT) {
		assertFakeTraceroutePath(c, s.expectNetpath(c, agentHostname, func(np *aggregator.Netpath) bool {
			return np.Destination.Hostname == targetIP.String() && np.Protocol == "TCP" && np.Destination.Port == 80
		}), agentHostname, routerIP, targetIP, 80, "")
		assertFakeTraceroutePath(c, s.expectNetpath(c, agentHostname, func(np *aggregator.Netpath) bool {
			return np.Destination.Hostname == targetIP.String() && np.Protocol == "TCP" && np.Destination.Port == 8080
		}), agentHostname, routerIP, targetIP, 8080, "")
		assertFakeTraceroutePath(c, s.expectNetpath(c, agentHostname, func(np *aggregator.Netpath) bool {
			return np.Destination.Hostname == targetIP.String() && np.Protocol == "UDP" && np.Destination.Port == 0
		}), agentHostname, routerIP, targetIP, 0, networkPathTestConfigID)
		assertFakeTraceroutePath(c, s.expectNetpath(c, agentHostname, func(np *aggregator.Netpath) bool {
			return np.Destination.Hostname == targetIP.String() && np.Protocol == "TCP" && np.Destination.Port == 443
		}), agentHostname, routerIP, targetIP, 443, networkPathTestConfigID)
	}, 5*time.Minute, 3*time.Second)

	s.deleteRemoteConfig(t, fakeIntakeClient)

	s.EventuallyWithT(func(c *assert.CollectT) {
		entries := s.networkPathConfigCheckEntries(c)
		assertNetworkPathConfigCount(c, entries, isLocalNetworkPathSource, 2)
		assertNetworkPathConfigCount(c, entries, isRemoteConfigNetworkPathSource, 0)
	}, 3*time.Minute, 5*time.Second)
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

func assertFakeTraceroutePath(c *assert.CollectT, np *aggregator.Netpath, agentHostname string, routerIP, targetIP net.IP, expectedPort uint16, expectedTestConfigID string) {
	assertPayloadBase(c, np, agentHostname)
	assert.Equal(c, payload.SourceProductNetworkPath, np.SourceProduct)
	assert.Equal(c, targetIP.String(), np.Destination.Hostname)
	assert.Equal(c, expectedPort, np.Destination.Port)
	assert.Equal(c, expectedTestConfigID, np.TestConfigID)

	require.Len(c, np.Traceroute.Runs, 1)
	run := np.Traceroute.Runs[0]
	assert.NotEmpty(c, run.RunID)
	assert.NotEmpty(c, run.Source.IPAddress)
	assert.NotZero(c, run.Source.Port)
	assert.Equal(c, targetIP, run.Destination.IPAddress)

	require.Len(c, run.Hops, 2)
	assert.Equal(c, 1, run.Hops[0].TTL)
	assert.Equal(c, routerIP, run.Hops[0].IPAddress)
	assert.True(c, run.Hops[0].Reachable)
	assert.Equal(c, 2, run.Hops[1].TTL)
	assert.Equal(c, targetIP, run.Hops[1].IPAddress)
	assert.True(c, run.Hops[1].Reachable)

	require.Len(c, np.E2eProbe.RTTs, 1)
	assert.Equal(c, 1, np.E2eProbe.PacketsSent)
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
