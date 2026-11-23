// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inframode

import (
	_ "embed"
	"fmt"
	"slices"
	"strings"
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

//go:embed fixtures/custom_infra_mode_events.py
var customInfraModeEventsPython []byte

const (
	checkEventsSource     = "custom_infra_mode_events"
	checkEventsMetricName = "custom_infra_mode_events.metric"
	checkEventsTag        = "e2e:infra_mode_check_events"
	checkEventsPythonPath = "/etc/datadog-agent/checks.d/custom_infra_mode_events.py"
	checkEventsConfig     = `
init_config:
instances:
  - {}
`
)

// checkEventsSuite asserts infra_mode on a check-submitted event in fakeintake,
// and that the same check's custom_* metric stays unmarked.
type checkEventsSuite struct {
	e2e.BaseSuite[environments.Host]
	infraMode  string
	expectMark bool
	markTag    string
}

func (s *checkEventsSuite) getSuiteOptions(stackName string) []e2e.SuiteOption {
	agentConfig := fmt.Sprintf(`
infrastructure_mode: %s
logs_enabled: false
apm_config:
  enabled: false
process_config:
  enabled: false
`, s.infraMode)

	return []e2e.SuiteOption{
		e2e.WithStackName(stackName),
		e2e.WithProvisioner(
			awshost.Provisioner(
				awshost.WithRunOptions(
					scenec2.WithAgentOptions(
						agentparams.WithAgentConfig(agentConfig),
						agentparams.WithIntegration(checkEventsSource+".d", checkEventsConfig),
						agentparams.WithFile(checkEventsPythonPath, string(customInfraModeEventsPython), true),
					),
				),
			),
		),
	}
}

func (s *checkEventsSuite) TestCheckEventInfraMode() {
	fakeintake := s.Env().FakeIntake.Client()

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		events, err := fakeintake.FilterEvents(
			checkEventsSource,
			client.WithTags[*aggregator.Event]([]string{checkEventsTag}),
		)
		require.NoError(c, err)
		require.NotEmpty(c, events, "no %s events yet", checkEventsSource)

		if s.expectMark {
			marked := slices.ContainsFunc(events, func(e *aggregator.Event) bool {
				return slices.Contains(e.GetTags(), s.markTag)
			})
			assert.Truef(c, marked, "no %s event tagged %s among %d events", checkEventsSource, s.markTag, len(events))
			return
		}

		for _, e := range events {
			for _, tag := range e.GetTags() {
				assert.Falsef(c, strings.HasPrefix(tag, "infra_mode:"),
					"unexpected infra_mode tag %q on %s event under infrastructure_mode=%s", tag, checkEventsSource, s.infraMode)
			}
		}
	}, 5*time.Minute, 15*time.Second, "check event infra_mode mark did not match expectation in fakeintake")

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		metrics, err := fakeintake.FilterMetrics(
			checkEventsMetricName,
			client.WithTags[*aggregator.MetricSeries]([]string{checkEventsTag}),
			client.WithMetricValueHigherThan(0),
		)
		require.NoError(c, err)
		require.NotEmpty(c, metrics, "no %s metrics yet", checkEventsMetricName)

		for _, m := range metrics {
			for _, tag := range m.GetTags() {
				assert.Falsef(c, strings.HasPrefix(tag, "infra_mode:"),
					"%s from custom check must not carry infra_mode tag %q", checkEventsMetricName, tag)
			}
		}
	}, 3*time.Minute, 10*time.Second, "custom check metric did not reach fakeintake unmarked")
}
