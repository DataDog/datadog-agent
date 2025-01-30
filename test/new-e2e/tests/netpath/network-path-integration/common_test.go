// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package netpath contains e2e tests for Network Path Integration feature
package networkpathintegration

import (
	_ "embed"
	"fmt"

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
