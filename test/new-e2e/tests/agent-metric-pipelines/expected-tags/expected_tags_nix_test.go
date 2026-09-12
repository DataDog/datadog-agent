// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package expectedtags contains e2e tests for the expected_tags_duration feature.
package expectedtags

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/fakeintake/client"
)

const (
	// hostTag is declared under `tags:` in datadog.yaml, so it is a host tag: nothing but
	// expected_tags_duration puts it on a metric payload.
	hostTag = "e2e_expected_tags:present"

	gaugeMetric = "e2e.expectedtags.gauge"
	distMetric  = "e2e.expectedtags.dist"
)

// baseAgentConfig keeps the tagging window open for the whole suite. The deadline is anchored
// to Agent process start, which happens during provisioning, so it has to outlast that too.
const baseAgentConfig = `
expected_tags_duration: 30m
tags:
  - ` + hostTag + `
`

// v1AgentConfig ships series as JSON on /api/v1/series.
const v1AgentConfig = baseAgentConfig + `
use_v2_api:
  series: false
`

// v2AgentConfig is the default serializer: series on /api/v2/series, sketches on
// /api/beta/sketches.
const v2AgentConfig = baseAgentConfig

// v3AgentConfig ships series over the v3 intake. The default is `datadog_only`, and the
// fakeintake URL is not a Datadog URL, so without this the Agent stays on v2.
const v3AgentConfig = baseAgentConfig + `
use_v3_api:
  series:
    enabled: "true"
`

// expectedTagsSuite holds everything the intake variants share. Each entry point below declares
// its own type: BaseSuite derives the Pulumi stack name from the struct type name, so two entry
// points sharing a type would race on one stack.
type expectedTagsSuite struct {
	e2e.BaseSuite[environments.Host]
}

type expectedTagsV1Suite struct {
	expectedTagsSuite
}

type expectedTagsV2Suite struct {
	expectedTagsSuite
}

type expectedTagsV3Suite struct {
	expectedTagsSuite
}

func runSuite[T e2e.Suite[environments.Host]](t *testing.T, suite T, stackName, agentConfig string) {
	t.Helper()
	t.Parallel()

	e2e.Run(t, suite,
		e2e.WithProvisioner(
			awshost.Provisioner(
				awshost.WithRunOptions(
					scenec2.WithAgentOptions(agentparams.WithAgentConfig(agentConfig)),
				),
			),
		),
		e2e.WithStackName(stackName),
	)
}

// TestExpectedTagsDurationV1Intake covers series on the v1 intake.
func TestExpectedTagsDurationV1Intake(t *testing.T) {
	runSuite(t, &expectedTagsV1Suite{}, "expectedtags-v1", v1AgentConfig)
}

// TestExpectedTagsDurationV2Intake covers series and sketches on the v2 intake.
func TestExpectedTagsDurationV2Intake(t *testing.T) {
	runSuite(t, &expectedTagsV2Suite{}, "expectedtags-v2", v2AgentConfig)
}

// TestExpectedTagsDurationV3Intake covers series on the v3 intake.
func TestExpectedTagsDurationV3Intake(t *testing.T) {
	runSuite(t, &expectedTagsV3Suite{}, "expectedtags-v3", v3AgentConfig)
}

// sendStatsd emits one DogStatsD payload over UDP from the remote host.
func (s *expectedTagsSuite) sendStatsd(c *assert.CollectT, payload string) {
	s.Env().RemoteHost.MustExecuteOn(c, fmt.Sprintf(`bash -c 'echo -n "%s" > /dev/udp/127.0.0.1/8125'`, payload))
}

// TestHostTagsOnSeries keeps a gauge flowing until a flush delivers it to the intake carrying
// the host tag. FilterMetrics merges the v1, v2 and v3 series endpoints, so the same assertion
// covers whichever encoder this suite's config selected.
func (s *expectedTagsSuite) TestHostTagsOnSeries() {
	s.EventuallyWithT(func(c *assert.CollectT) {
		s.sendStatsd(c, gaugeMetric+":1|g")

		metrics, err := s.Env().FakeIntake.Client().FilterMetrics(gaugeMetric,
			client.WithTags[*aggregator.MetricSeries]([]string{hostTag}))
		require.NoError(c, err)
		assert.NotEmpty(c, metrics, "no %s series tagged %q has reached the intake yet", gaugeMetric, hostTag)
	}, 5*time.Minute, 10*time.Second)
}

// TestHostTagsOnSketches covers the sketch sink, which is tagged separately from series in
// createIterableMetrics. Only the v2 suite runs it: sketches have one intake version today.
func (s *expectedTagsV2Suite) TestHostTagsOnSketches() {
	s.EventuallyWithT(func(c *assert.CollectT) {
		s.sendStatsd(c, distMetric+":1|d")

		sketches, err := s.Env().FakeIntake.Client().FilterSketches(distMetric,
			client.WithTags[*aggregator.Sketch]([]string{hostTag}))
		require.NoError(c, err)
		assert.NotEmpty(c, sketches, "no %s sketch tagged %q has reached the intake yet", distMetric, hostTag)
	}, 5*time.Minute, 10*time.Second)
}
