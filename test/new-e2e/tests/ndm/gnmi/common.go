// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package gnmi

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
)

func assertMetricHasTags(c *assert.CollectT, metrics []*aggregator.MetricSeries, deviceIP, interfaceName string) {
	if !assert.NotEmpty(c, metrics, "expected at least one metric sample") {
		return
	}

	tags := metrics[0].Tags
	assert.Contains(c, tags, "device_ip:"+deviceIP, "metric tags %v missing device_ip", tags)
	assert.Contains(c, tags, "interface:"+interfaceName, "metric tags %v missing interface", tags)
}

func requireMetricHasTags(t *testing.T, metrics []*aggregator.MetricSeries, deviceIP, interfaceName string) {
	require.NotEmpty(t, metrics, "expected at least one metric sample")

	tags := metrics[0].Tags
	require.Contains(t, tags, "device_ip:"+deviceIP, "metric tags %v missing device_ip", tags)
	require.Contains(t, tags, "interface:"+interfaceName, "metric tags %v missing interface", tags)
}
