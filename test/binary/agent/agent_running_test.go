// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agent holds binary tests that run the core Agent against a fakeintake. The services
// are declared in BUILD.bazel and started by rules_itest before the test runs.
package agent

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	metricspb "github.com/DataDog/agent-payload/v5/gogen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
)

const (
	agentRunningMetric = "datadog.agent.running"
	hostname           = "binary-test-host"
)

// TestAgentRunning checks the datadog.agent.running gauge the aggregator appends to every flush.
func TestAgentRunning(t *testing.T) {
	fi := fakeintake.NewClient("http://127.0.0.1:" + assignedPort(t, "@@//test/binary/agent:fakeintake"))

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		series, err := fi.FilterMetrics(agentRunningMetric)
		require.NoError(c, err)
		require.NotEmpty(c, series, "no %s series has reached the intake yet", agentRunningMetric)
		for _, m := range series {
			assert.Equal(c, metricspb.MetricPayload_GAUGE, m.Type)
			assert.Equal(c, hostname, seriesHost(m))
			assert.True(c, slices.ContainsFunc(m.Tags, func(t string) bool { return strings.HasPrefix(t, "version:") }),
				"%s must carry the Agent version tag, got %v", agentRunningMetric, m.Tags)
			for _, p := range m.Points {
				assert.Equal(c, 1.0, p.Value)
			}
		}
	}, 2*time.Minute, time.Second)
}

// assignedPort returns the port rules_itest assigned to the service with the given label.
func assignedPort(t *testing.T, label string) string {
	var ports map[string]string
	require.NoError(t, json.Unmarshal([]byte(os.Getenv("ASSIGNED_PORTS")), &ports), "run this test through its service_test target")
	port, ok := ports[label]
	require.True(t, ok, "no port assigned to %s in %v", label, ports)
	return port
}

func seriesHost(m *aggregator.MetricSeries) string {
	for _, r := range m.Resources {
		if r.Type == "host" {
			return r.Name
		}
	}
	return ""
}
