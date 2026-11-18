// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package vdi tests the deployed Windows VDI check and its system-probe inventory.
package vdi

import (
	"flag"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenwindows "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2/windows"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	winawshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host/windows"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
)

var envDescriptor = flag.String("vdi-env-descriptor", "", "existing Windows host/fakeintake resource JSON; skips cloud provisioning")

const (
	agentConfig  = "process_manager:\n  enabled: false\nvdi:\n  enabled: true\n"
	checkConfig  = "init_config:\ninstances:\n  - provider: azure_virtual_desktop\n    inventory_stale_ttl: 0\n"
	healthCheck  = "vdi.session_enrichment.health"
	probeService = "datadog-system-probe"
)

var inventoryTags = []string{"vdi_provider:azure_virtual_desktop", "vdi_protocol:rdp"}
var aggregateMetrics = []string{"vdi.session.count", "vdi.session.active", "vdi.session.disconnected"}

type vdiWindowsSuite struct {
	e2e.BaseSuite[environments.WindowsHost]
}

func TestVDIWindows(t *testing.T) {
	t.Parallel()
	var p provisioners.Provisioner
	if *envDescriptor != "" {
		p = provisioners.NewStaticStackProvisioner[environments.WindowsHost]("", *envDescriptor)
	} else {
		p = winawshost.Provisioner(winawshost.WithRunOptions(scenwindows.WithAgentOptions(
			agentparams.WithAgentConfig(agentConfig),
			agentparams.WithIntegration("vdi.d", checkConfig),
		)))
	}
	e2e.Run(t, &vdiWindowsSuite{}, e2e.WithProvisioner(p))
}

func (s *vdiWindowsSuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)
	require.NoError(s.T(), s.Env().FakeIntake.Client().FlushServerAndResetAggregators())
}

func (s *vdiWindowsSuite) latestHealth(c *assert.CollectT, status int) int64 {
	runs, err := s.Env().FakeIntake.Client().FilterCheckRuns(healthCheck,
		fakeintakeclient.WithTags[*aggregator.CheckRun](inventoryTags))
	require.NoError(c, err)
	var latest int64
	for _, run := range runs {
		if run.Status == status && int64(run.Timestamp) > latest {
			latest = int64(run.Timestamp)
		}
	}
	return latest
}

func (s *vdiWindowsSuite) assertMetricsAfter(c *assert.CollectT, after int64, expected bool) {
	for _, name := range aggregateMetrics {
		series, err := s.Env().FakeIntake.Client().FilterMetrics(name,
			fakeintakeclient.WithTags[*aggregator.MetricSeries](inventoryTags))
		require.NoError(c, err)
		found := false
		for _, metric := range series {
			for _, point := range metric.Points {
				if point.Timestamp > after {
					found = true
					assert.GreaterOrEqual(c, point.Value, 0.0, "%s must be nonnegative", name)
				}
			}
		}
		assert.Equal(c, expected, found, "%s samples newer than %d", name, after)
	}
}

func (s *vdiWindowsSuite) TestScheduledInventoryFailureAndRecovery() {
	var healthyAt int64
	s.EventuallyWithT(func(c *assert.CollectT) {
		healthyAt = s.latestHealth(c, 0)
		require.Positive(c, healthyAt, "scheduled VDI check must report healthy inventory")
		s.assertMetricsAfter(c, 0, true)
	}, 5*time.Minute, 5*time.Second)

	s.T().Cleanup(func() { require.NoError(s.T(), common.StartService(s.Env().RemoteHost, probeService)) })
	require.NoError(s.T(), common.StopService(s.Env().RemoteHost, probeService))

	var firstFailure, lastFailure int64
	s.EventuallyWithT(func(c *assert.CollectT) {
		if lastFailure == 0 {
			observed := s.latestHealth(c, 2)
			require.Greater(c, observed, healthyAt, "unavailable inventory must report critical health")
			if firstFailure == 0 {
				firstFailure = observed
			}
			// Freeze the second unhealthy timestamp before waiting for the
			// independently scheduled control metric to cross this boundary.
			require.GreaterOrEqual(c, observed-firstFailure, int64(15), "observe multiple unhealthy collection intervals")
			lastFailure = observed
		}
		// Check runs and metric series use independent forwarding endpoints. An
		// ordinary metric confirms the metric stream has also crossed this window.
		control, err := s.Env().FakeIntake.Client().FilterMetrics("system.uptime")
		require.NoError(c, err)
		metricsFlushed := false
		for _, metric := range control {
			for _, point := range metric.Points {
				metricsFlushed = metricsFlushed || point.Timestamp >= lastFailure
			}
		}
		require.True(c, metricsFlushed, "wait for the metric forwarding stream to cross the failure window")
		s.assertMetricsAfter(c, firstFailure, false)
	}, 2*time.Minute, 5*time.Second)

	require.NoError(s.T(), common.StartService(s.Env().RemoteHost, probeService))
	s.EventuallyWithT(func(c *assert.CollectT) {
		require.Greater(c, s.latestHealth(c, 0), lastFailure, "recovery must emit newly timestamped healthy inventory")
		s.assertMetricsAfter(c, lastFailure, true)
	}, 2*time.Minute, 5*time.Second)
}
