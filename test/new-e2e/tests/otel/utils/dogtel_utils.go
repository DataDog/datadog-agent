// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package utils contains util functions for OTel e2e tests
package utils

import (
	"fmt"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
)

const (
	// DogtelLivenessMetricName is the metric emitted by the dogtelextension on every Start()
	DogtelLivenessMetricName = "otel.dogtel_extension.running"
)

// TestDogtelLivenessMetric verifies that the dogtelextension emits its liveness gauge metric.
// The metric is sent on Start() when otel_standalone=true and should appear in the fake intake.
func TestDogtelLivenessMetric(s OTelTestSuite) {
	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	require.NoError(s.T(), err)

	s.T().Log("Waiting for dogtel liveness metric:", DogtelLivenessMetricName)
	var metrics []*aggregator.MetricSeries
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		metrics, err = s.Env().FakeIntake.Client().FilterMetrics(DogtelLivenessMetricName)
		assert.NoError(c, err)
		assert.NotEmpty(c, metrics)
	}, 5*time.Minute, 10*time.Second)

	require.NotEmpty(s.T(), metrics)
	s.T().Log("Got dogtel liveness metric:", metrics[0])

	// The metric should be a gauge with value 1.0
	m := metrics[0]
	require.NotEmpty(s.T(), m.Points)
	assert.Equal(s.T(), 1.0, m.Points[0].Value, "otel.dogtel_extension.running should always be 1.0")
}

// TestDogtelTaggerServerRunning verifies that the dogtel tagger gRPC server is listening
// on the expected port inside the otel-agent container.
func TestDogtelTaggerServerRunning(s OTelTestSuite, port int) {
	agent := getAgentPod(s)
	portHex := fmt.Sprintf("%04X", port)
	// /proc/net/tcp6 lists listening sockets; the local address column is ADDR:PORT
	// where PORT is the port in uppercase hex (big-endian).
	cmd := []string{
		"/bin/sh", "-c",
		// /proc/net/tcp format: "XXXXXXXX:PPPP XXXXXXXX:PPPP STATE ..."
		// The port appears as ":PPPP " (colon before, space after), not " PPPP ".
		fmt.Sprintf("grep -i ':%s ' /proc/net/tcp6 /proc/net/tcp 2>/dev/null | grep ' 0A '", portHex),
	}
	s.T().Logf("Checking that tagger gRPC server is listening on port %d (0x%s)", port, portHex)
	stdout, _, err := s.Env().KubernetesCluster.KubernetesClient.PodExec("datadog", agent.Name, "otel-agent", cmd)
	require.NoError(s.T(), err, "Tagger gRPC server should be listening on port %d", port)
	require.NotEmpty(s.T(), stdout, "Expected a LISTEN entry for port %d in /proc/net/tcp", port)
}
