// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
)

// OTelDockerTestSuite is an interface for the OTel e2e test suite.
type OTelDockerTestSuite interface {
	T() *testing.T
	Env() *environments.DockerHost
}

// TestCalendarAppDocker verifies the calendar app is sending telemetry for e2e tests
func TestCalendarAppDocker(s OTelDockerTestSuite) {
	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	require.NoError(s.T(), err)

	s.T().Log("Waiting for calendar app to start")
	// Wait for calendar app to start
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		logs, err := s.Env().FakeIntake.Client().FilterLogs(CalendarService, fakeintake.WithMessageContaining(log2Body))
		assert.NoError(c, err)
		assert.NotEmpty(c, logs)
	}, 30*time.Minute, 10*time.Second)
}

// TestTracesDocker tests that OTLP traces are received through OTel pipelines as expected
func TestTracesDocker(s OTelDockerTestSuite) {
	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	require.NoError(s.T(), err)

	var traces []*aggregator.TracePayload
	s.T().Log("Waiting for traces")
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		traces, err = s.Env().FakeIntake.Client().GetTraces()
		if !assert.NoError(c, err) {
			return
		}
		if !assert.NotEmpty(c, traces) {
			return
		}
		trace := traces[0]
		if !assert.NotEmpty(s.T(), trace.TracerPayloads) {
			return
		}
		tp := trace.TracerPayloads[0]
		if !assert.NotEmpty(s.T(), tp.Chunks) {
			return
		}
		if !assert.NotEmpty(s.T(), tp.Chunks[0].Spans) {
			return
		}
		assert.Equal(s.T(), CalendarService, tp.Chunks[0].Spans[0].Service)
	}, 5*time.Minute, 10*time.Second)
	require.NotEmpty(s.T(), traces)
	s.T().Log("Got traces", s.T().Name(), traces)

	// Verify tags on traces and spans
	tp := traces[0].TracerPayloads[0]
	assert.Equal(s.T(), env, tp.Env)
	assert.Equal(s.T(), version, tp.AppVersion)
	require.NotEmpty(s.T(), tp.Chunks)
	require.NotEmpty(s.T(), tp.Chunks[0].Spans)
	spans := tp.Chunks[0].Spans
	for _, sp := range spans {
		assert.Equal(s.T(), CalendarService, sp.Service)
		assert.Equal(s.T(), env, sp.Meta["env"])
		assert.Equal(s.T(), version, sp.Meta["version"])
		assert.Equal(s.T(), customAttributeValue, sp.Meta[customAttribute])
		if sp.Meta["span.kind"] == "client" {
			assert.Equal(s.T(), "calendar-rest-go", sp.Meta["otel.library.name"])
		} else {
			assert.Equal(s.T(), "go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp", sp.Meta["otel.library.name"])
		}
		assert.Equal(s.T(), traces[0].HostName, tp.Hostname)
	}
}

// TestMetricsDocker tests that OTLP metrics are received through OTel pipelines as expected
func TestMetricsDocker(s OTelDockerTestSuite) {
	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	require.NoError(s.T(), err)

	var metrics []*aggregator.MetricSeries
	s.T().Log("Waiting for metrics")
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		tags := []string{fmt.Sprintf("service:%v", CalendarService)}
		metrics, err = s.Env().FakeIntake.Client().FilterMetrics("calendar-rest-go.api.counter", fakeintake.WithTags[*aggregator.MetricSeries](tags))
		assert.NoError(c, err)
		assert.NotEmpty(c, metrics)
	}, 5*time.Minute, 10*time.Second)
	s.T().Log("Got metrics", s.T().Name(), metrics)

	for _, metricSeries := range metrics {
		tags := getTagMapFromSlice(s.T(), metricSeries.Tags)
		assert.Equal(s.T(), CalendarService, tags["service"])
		assert.Equal(s.T(), env, tags["env"])
		assert.Equal(s.T(), version, tags["version"])
		assert.Equal(s.T(), customAttributeValue, tags[customAttribute])
		hasHostResource := false
		for _, resource := range metricSeries.Resources {
			if resource.Type == "host" {
				assert.NotNil(s.T(), resource.Name)
				hasHostResource = true
			}
		}
		assert.True(s.T(), hasHostResource)
	}
}

// TestLogsDocker tests that OTLP logs are received through OTel pipelines as expected
func TestLogsDocker(s OTelDockerTestSuite) {
	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	require.NoError(s.T(), err)

	var logs []*aggregator.Log
	s.T().Log("Waiting for logs")
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		logs, err = s.Env().FakeIntake.Client().FilterLogs(CalendarService, fakeintake.WithMessageContaining(log2Body))
		assert.NoError(c, err)
		assert.NotEmpty(c, logs)
	}, 5*time.Minute, 10*time.Second)
	for _, l := range logs {
		s.T().Log("Got log", l)
	}

	require.NotEmpty(s.T(), logs)
	for _, log := range logs {
		tags := getLogTagsAndAttrs(s.T(), log)
		assert.Contains(s.T(), log.Message, log2Body)
		assert.Equal(s.T(), CalendarService, tags["service"])
		assert.Equal(s.T(), env, tags["env"])
		assert.Equal(s.T(), version, tags["version"])
		assert.Equal(s.T(), customAttributeValue, tags[customAttribute])
		assert.NotNil(s.T(), log.HostName)
	}
}
