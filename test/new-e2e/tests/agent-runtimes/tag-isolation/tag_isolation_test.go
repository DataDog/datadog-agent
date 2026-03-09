// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tagisolation contains e2e tests that verify custom tags assigned to
// one check instance do not leak into another instance of the same check.
//
// This validates the end-to-end pipeline from multi-instance check config
// through the aggregator and serializer to fakeintake. It uses a custom Python
// check for simplicity and determinism. Note that Go-specific regressions
// (e.g. missing BuildID causing shared senders) require a Go check to reproduce
// and are covered by unit tests in the respective check packages.
package tagisolation

import (
	_ "embed"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	ec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/fakeintake/client"
)

//go:embed fixtures/tag_check.py
var tagCheckPython string

// tagCheckConfig defines two instances of the same check, each with a distinct
// tag and metric value. If tag isolation is broken, metrics from one instance
// will carry the other instance's tag.
const tagCheckConfig = `init_config:

instances:
  - metric_value: 100
    tags:
      - instance:alpha
  - metric_value: 200
    tags:
      - instance:beta
`

type tagIsolationSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestTagIsolationSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &tagIsolationSuite{}, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(
				ec2.WithAgentOptions(
					agentparams.WithIntegration("tag_check.d", tagCheckConfig),
					agentparams.WithFile("/etc/datadog-agent/checks.d/tag_check.py", tagCheckPython, true),
				),
			),
		),
	))
}

// TestAlphaInstanceMetrics verifies that the alpha instance reports metrics
// tagged with instance:alpha and value 100, without the beta tag.
func (s *tagIsolationSuite) TestAlphaInstanceMetrics() {
	fakeintake := s.Env().FakeIntake.Client()

	s.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := fakeintake.FilterMetrics("tag_check.metric",
			client.WithTags[*aggregator.MetricSeries]([]string{"instance:alpha"}),
		)
		require.NoError(c, err)
		require.NotEmpty(c, metrics, "no 'tag_check.metric' with instance:alpha yet")

		// Verify the tag is exclusively alpha — no beta tag present.
		for _, m := range metrics {
			for _, tag := range m.Tags {
				require.NotEqual(c, "instance:beta", tag,
					"instance:beta tag leaked into alpha instance metric")
			}
		}

		// Verify the metric value matches the alpha config (100).
		latest := metrics[len(metrics)-1]
		require.NotEmpty(c, latest.Points, "metric has no data points")
		require.InDelta(c, 100, latest.Points[len(latest.Points)-1].Value, 0.1,
			"alpha instance should report value 100")
	}, 3*time.Minute, 10*time.Second)
}

// TestBetaInstanceMetrics verifies that the beta instance reports metrics
// tagged with instance:beta and value 200, without the alpha tag.
func (s *tagIsolationSuite) TestBetaInstanceMetrics() {
	fakeintake := s.Env().FakeIntake.Client()

	s.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := fakeintake.FilterMetrics("tag_check.metric",
			client.WithTags[*aggregator.MetricSeries]([]string{"instance:beta"}),
		)
		require.NoError(c, err)
		require.NotEmpty(c, metrics, "no 'tag_check.metric' with instance:beta yet")

		// Verify the tag is exclusively beta — no alpha tag present.
		for _, m := range metrics {
			for _, tag := range m.Tags {
				require.NotEqual(c, "instance:alpha", tag,
					"instance:alpha tag leaked into beta instance metric")
			}
		}

		// Verify the metric value matches the beta config (200).
		latest := metrics[len(metrics)-1]
		require.NotEmpty(c, latest.Points, "metric has no data points")
		require.InDelta(c, 200, latest.Points[len(latest.Points)-1].Value, 0.1,
			"beta instance should report value 200")
	}, 3*time.Minute, 10*time.Second)
}

// TestBothInstancesReportServiceChecks verifies that both instances report
// their service checks independently.
func (s *tagIsolationSuite) TestBothInstancesReportServiceChecks() {
	fakeintake := s.Env().FakeIntake.Client()

	s.EventuallyWithT(func(c *assert.CollectT) {
		checks, err := fakeintake.FilterCheckRuns("tag_check.can_connect")
		require.NoError(c, err)
		require.NotEmpty(c, checks, "no 'tag_check.can_connect' service checks yet")

		// Verify per-check-run tag exclusivity: each check run must belong
		// to exactly one instance, and we must see both instances across all
		// check runs. A leaked tag (both instance:alpha and instance:beta on
		// a single check run) must fail the test.
		hasAlpha := false
		hasBeta := false
		for _, cr := range checks {
			crHasAlpha := false
			crHasBeta := false
			for _, tag := range cr.Tags {
				if tag == "instance:alpha" {
					crHasAlpha = true
				}
				if tag == "instance:beta" {
					crHasBeta = true
				}
			}
			require.False(c, crHasAlpha && crHasBeta,
				"service check has both instance:alpha and instance:beta — tag isolation broken")
			if crHasAlpha {
				hasAlpha = true
			}
			if crHasBeta {
				hasBeta = true
			}
			// Each service check should be OK (status 0).
			require.EqualValues(c, 0, cr.Status,
				"expected service check OK (0), got %d", cr.Status)
		}
		require.True(c, hasAlpha, "no service check from alpha instance")
		require.True(c, hasBeta, "no service check from beta instance")
	}, 3*time.Minute, 10*time.Second)
}
