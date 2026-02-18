// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package netpath contains e2e tests for Network Path Integration feature
package networkpathintegration

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
)

//go:embed fixtures/system-probe.yaml
var sysProbeConfig []byte

//go:embed fixtures/network_path.yaml
var networkPathIntegration []byte

var testAgentRunningMetricTagsTCP = []string{"protocol:TCP"}
var testAgentRunningMetricTagsUDP = []string{"protocol:UDP"}

func isNetpathDebugMode() bool {
	val, exist := os.LookupEnv("DD_E2E_TEST_NETPATH_DEBUG")
	return exist && val == "true"
}

type baseNetworkPathIntegrationTestSuite struct {
	e2e.BaseSuite[environments.Host]
}

func assertMetrics(fakeIntake *components.FakeIntake, c *assert.CollectT, metricTags [][]string) {
	fakeClient := fakeIntake.Client()

	metrics, err := fakeClient.FilterMetrics("datadog.network_path.path.monitored")
	require.NoError(c, err)
	assert.NotEmpty(c, metrics)
	for _, tags := range metricTags {
		// assert destination is monitored
		metrics, err = fakeClient.FilterMetrics("datadog.network_path.path.monitored", fakeintakeclient.WithTags[*aggregator.MetricSeries](tags))
		assert.NoError(c, err)
		assert.NotEmpty(c, metrics, fmt.Sprintf("metric with tags `%v` not found", tags))
	}
}

func (s *baseNetworkPathIntegrationTestSuite) findNetpath(isMatch func(*aggregator.Netpath) bool) (*aggregator.Netpath, error) {
	nps, err := s.Env().FakeIntake.Client().GetLatestNetpathEvents()
	if err != nil {
		return nil, err
	}
	if nps == nil {
		return nil, errors.New("GetLatestNetpathEvents() returned nil netpaths")
	}

	var match *aggregator.Netpath
	for _, np := range nps {
		if isMatch(np) {
			match = np
		}
	}
	return match, nil
}
func (s *baseNetworkPathIntegrationTestSuite) expectNetpath(c *assert.CollectT, isMatch func(*aggregator.Netpath) bool) *aggregator.Netpath {
	np, err := s.findNetpath(isMatch)
	require.NoError(c, err)

	require.NotNil(c, np, "could not find matching netpath")
	return np
}

func assertPayloadBase(c *assert.CollectT, np *aggregator.Netpath, hostname string) {
	if isNetpathDebugMode() {
		// Print payloads when debug mode, this helps debugging tests during development time
		tcpPathJSON, err := json.Marshal(np)
		assert.NoError(c, err)
		fmt.Println("NETWORK PATH PAYLOAD: ", string(tcpPathJSON))
	}

	assert.Equal(c, payload.PathOrigin("network_path_integration"), np.Origin)
	assert.NotEmpty(c, np.TestRunID)
	assert.Equal(c, "default", np.Namespace)

	// check that the timestamp is reasonably close to the current time
	tolerance := time.Hour
	assert.Greater(c, np.Timestamp, time.Now().Add(-tolerance).UnixMilli())
	assert.Less(c, np.Timestamp, time.Now().Add(tolerance).UnixMilli())

	assert.Equal(c, hostname, np.Source.Hostname)
}

func (s *baseNetworkPathIntegrationTestSuite) checkDatadogEUTCP(c *assert.CollectT, agentHostname string) {
	np := s.expectNetpath(c, func(np *aggregator.Netpath) bool {
		return np.Destination.Hostname == "api.datadoghq.eu" && np.Protocol == "TCP"
	})
	assert.Equal(c, uint16(443), np.Destination.Port)

	assertPayloadBase(c, np, agentHostname)

	require.NotEmpty(c, np.Traceroute.Runs)
	assert.NotEmpty(c, np.Traceroute.Runs[0].Hops)
}

func (s *baseNetworkPathIntegrationTestSuite) checkGoogleDNSUDP(c *assert.CollectT, agentHostname string) {
	np := s.expectNetpath(c, func(np *aggregator.Netpath) bool {
		return np.Destination.Hostname == "8.8.8.8" && np.Protocol == "UDP"
	})
	assertPayloadBase(c, np, agentHostname)

	require.NotEmpty(c, np.Traceroute.Runs)
	assert.NotEmpty(c, np.Traceroute.Runs[0].Hops)
}

func (s *baseNetworkPathIntegrationTestSuite) checkGoogleTCPSocket(c *assert.CollectT, agentHostname string) {
	np := s.expectNetpath(c, func(np *aggregator.Netpath) bool {
		return np.Destination.Hostname == "8.8.8.8" && np.Protocol == "TCP"
	})

	assertPayloadBase(c, np, agentHostname)

	assert.NotZero(c, np.Destination.Port)
	require.NotEmpty(c, np.Traceroute.Runs)
	assert.NotEmpty(c, np.Traceroute.Runs[0].Hops)

	// assert that one of the hops is reachable
	run := np.Traceroute.Runs[0]
	countKnownHops := 0
	for _, hop := range run.Hops {
		if hop.Reachable {
			countKnownHops++
		}
	}
	// > 1 verifies that we have more than just the last hop known
	assert.True(c, countKnownHops > 1, "expected to find at least one hop that is reachable")
}

func (s *baseNetworkPathIntegrationTestSuite) checkGoogleTCPDisableWindowsDriver(c *assert.CollectT, agentHostname string) {
	np := s.expectNetpath(c, func(np *aggregator.Netpath) bool {
		return np.Destination.Hostname == "1.1.1.1" && np.Protocol == "TCP"
	})

	assertPayloadBase(c, np, agentHostname)

	assert.NotZero(c, np.Destination.Port)
	require.NotEmpty(c, np.Traceroute.Runs)
	assert.NotEmpty(c, np.Traceroute.Runs[0].Hops)

	// assert that one of the hops is reachable
	run := np.Traceroute.Runs[0]
	countKnownHops := 0
	for _, hop := range run.Hops {
		if hop.Reachable {
			countKnownHops++
		}
	}
	// > 1 verifies that we have more than just the last hop known
	assert.True(c, countKnownHops > 1, "expected to find at least one hop that is reachable")
}
