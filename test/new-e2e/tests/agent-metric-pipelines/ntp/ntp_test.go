// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ntp contains e2e tests for the NTP check
package ntp

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	ec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/fakeintake/client"
)

// ntpCheckConfig overrides the default 15-minute collection interval so the
// test doesn't have to wait that long.
const ntpCheckConfig = `init_config:

instances:
  - offset_threshold: 60
    min_collection_interval: 30
`

type ntpSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestNTPSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &ntpSuite{}, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(
				ec2.WithAgentOptions(
					agentparams.WithIntegration("ntp.d", ntpCheckConfig),
				),
			),
		),
	))
}

// TestNTPOffsetMetric verifies that the NTP check reports the ntp.offset
// metric with the source:ntp tag to fakeintake.
func (s *ntpSuite) TestNTPOffsetMetric() {
	fakeintake := s.Env().FakeIntake.Client()

	s.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := fakeintake.FilterMetrics("ntp.offset",
			client.WithTags[*aggregator.MetricSeries]([]string{"source:ntp"}),
		)
		assert.NoError(c, err)
		assert.NotEmpty(c, metrics, "no 'ntp.offset' metrics with source:ntp tag yet")
	}, 5*time.Minute, 10*time.Second)
}

// TestNTPServiceCheck verifies that the NTP check reports the ntp.in_sync
// service check. On a healthy VM the check should report OK (offset < 60s).
func (s *ntpSuite) TestNTPServiceCheck() {
	fakeintake := s.Env().FakeIntake.Client()

	s.EventuallyWithT(func(c *assert.CollectT) {
		checkRuns, err := fakeintake.FilterCheckRuns("ntp.in_sync")
		assert.NoError(c, err)
		assert.NotEmpty(c, checkRuns, "no 'ntp.in_sync' service check yet")

		// On a normal VM the clock should be in sync
		latest := checkRuns[len(checkRuns)-1]
		assert.EqualValues(c, 0, latest.Status, "expected ntp.in_sync status OK (0), got %d", latest.Status)
	}, 5*time.Minute, 10*time.Second)
}
