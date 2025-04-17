// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package netpath contains e2e tests for Network Path Integration feature
package networkpathintegration

import (
	_ "embed"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
)

//go:embed fixtures/system-probe.yaml
var sysProbeConfig []byte

//go:embed fixtures/network_path.yaml
var networkPathIntegration []byte

var testAgentRunningMetricTagsTCP = []string{"destination_hostname:api.datadoghq.eu", "protocol:TCP", "destination_port:443"}
var testAgentRunningMetricTagsUDP = []string{"destination_hostname:8.8.8.8", "protocol:UDP"}

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

		// assert hops
		metrics, err = fakeClient.FilterMetrics("datadog.network_path.path.hops",
			fakeintakeclient.WithTags[*aggregator.MetricSeries](tags),
			fakeintakeclient.WithMetricValueHigherThan(0),
		)
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
		return nil, fmt.Errorf("GetLatestNetpathEvents() returned nil netpaths")
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
	assert.Equal(c, payload.PathOrigin("network_path_integration"), np.Origin)
	assert.NotEmpty(c, np.PathtraceID)
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

	assert.NotEmpty(c, np.Hops)
}

func (s *baseNetworkPathIntegrationTestSuite) checkGoogleDNSUDP(c *assert.CollectT, agentHostname string) {
	np := s.expectNetpath(c, func(np *aggregator.Netpath) bool {
		return np.Destination.Hostname == "8.8.8.8" && np.Protocol == "UDP"
	})
	assert.NotZero(c, np.Destination.Port)

	assertPayloadBase(c, np, agentHostname)

	assert.NotEmpty(c, np.Hops)
}
