// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package baseline contains e2e tests checking that DogStatsD series and distributions reach the
// intake intact under each metrics intake configuration.
package baseline

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	metricspb "github.com/DataDog/agent-payload/v5/gogen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
)

// baselineCase is one Agent configuration the metrics must be delivered intact under.
type baselineCase struct {
	// name is the subtest name and the stack-name suffix.
	name        string
	agentConfig string
}

// baselineCases is the test matrix. Which route each configuration selects is covered by unit
// tests in pkg/serializer; these only check what reaches the intake. The fakeintake URL is not a
// Datadog URL, so `datadog_only` behaves like `false` here.
//
// Other cases that fit as a single row: mixed series/distributions versions, v1 series
// (`use_v2_api.series: false`), or `serializer_compressor_kind: zlib`.
var baselineCases = []baselineCase{
	{
		name: "v2",
		agentConfig: `
use_v3_api:
  series:
    enabled: "false"
  distributions:
    enabled: "false"
`,
	},
	{
		name: "v3",
		agentConfig: `
use_v3_api:
  series:
    enabled: "true"
  distributions:
    enabled: "true"
`,
	},
}

const (
	gaugeMetric = "e2e.baseline.gauge"
	countMetric = "e2e.baseline.count"
	distMetric  = "e2e.baseline.dist"

	agentRunningMetric = "datadog.agent.running"

	gaugeValue = 42.0
	countValue = 3.0
	// flushInterval is the DogStatsD bucket width, which counts are divided by to become rates.
	flushInterval = 10

	attemptTagPrefix = "attempt:"
)

// distValues are sent in one packet, so they land in one context and are checked as one sketch.
var distValues = []float64{1, 2, 3, 10}

type baselineSuite struct {
	e2e.BaseSuite[environments.Host]

	attempt int
}

// TestBaseline runs every baselineCases row on its own stack.
func TestBaseline(t *testing.T) {
	t.Parallel()
	for _, tc := range baselineCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e2e.Run(t, &baselineSuite{},
				e2e.WithProvisioner(
					awshost.Provisioner(
						awshost.WithRunOptions(
							scenec2.WithAgentOptions(agentparams.WithAgentConfig(tc.agentConfig)),
						),
					),
				),
				e2e.WithStackName("metrics-baseline-"+tc.name),
			)
		})
	}
}

// TestSeries checks that a gauge and a count keep their type, value, interval and host on the
// wire.
func (s *baselineSuite) TestSeries() {
	hostname := s.Env().Agent.Client.Hostname()

	s.EventuallyWithT(func(c *assert.CollectT) {
		tag := s.nextAttemptTag()
		s.sendStatsd(c,
			fmt.Sprintf("%s:%g|g|#%s", gaugeMetric, gaugeValue, tag),
			fmt.Sprintf("%s:%g|c|#%s", countMetric, countValue, tag),
		)

		gauges, err := s.Env().FakeIntake.Client().FilterMetrics(gaugeMetric)
		require.NoError(c, err)
		require.NotEmpty(c, gauges, "no %s series has reached the intake yet", gaugeMetric)
		for _, m := range gauges {
			assert.Equal(c, metricspb.MetricPayload_GAUGE, m.Type)
			assert.Equal(c, hostname, seriesHost(m))
			for _, p := range m.Points {
				assert.Equal(c, gaugeValue, p.Value)
			}
		}

		counts, err := s.Env().FakeIntake.Client().FilterMetrics(countMetric)
		require.NoError(c, err)
		// Counts keep flushing zeros after their last sample, so only non-zero points carry
		// the value that was sent. Each attempt tag has a single sample.
		var sent int
		for _, m := range counts {
			assert.Equal(c, metricspb.MetricPayload_RATE, m.Type)
			assert.EqualValues(c, flushInterval, m.Interval)
			assert.Equal(c, hostname, seriesHost(m))
			for _, p := range m.Points {
				if p.Value != 0 {
					sent++
					assert.InDelta(c, countValue/flushInterval, p.Value, 1e-9)
				}
			}
		}
		assert.NotZero(c, sent, "no non-zero %s point has reached the intake yet", countMetric)
	}, 5*time.Minute, 10*time.Second)
}

// TestAgentRunning checks the datadog.agent.running gauge the aggregator appends to every flush.
func (s *baselineSuite) TestAgentRunning() {
	hostname := s.Env().Agent.Client.Hostname()

	s.EventuallyWithT(func(c *assert.CollectT) {
		series, err := s.Env().FakeIntake.Client().FilterMetrics(agentRunningMetric)
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
	}, 5*time.Minute, 10*time.Second)
}

// TestSketches checks that a distribution's count, sum, min, max, bins and host survive the
// wire.
func (s *baselineSuite) TestSketches() {
	hostname := s.Env().Agent.Client.Hostname()

	var wantSum float64
	for _, v := range distValues {
		wantSum += v
	}
	want := sketchTotals{
		cnt: int64(len(distValues)),
		sum: wantSum,
		min: distValues[0],
		max: distValues[len(distValues)-1],
	}

	s.EventuallyWithT(func(c *assert.CollectT) {
		tag := s.nextAttemptTag()
		lines := make([]string, 0, len(distValues))
		for _, v := range distValues {
			lines = append(lines, fmt.Sprintf("%s:%g|d|#%s", distMetric, v, tag))
		}
		s.sendStatsd(c, lines...)

		sketches, err := s.Env().FakeIntake.Client().FilterSketches(distMetric)
		require.NoError(c, err)

		// The samples of one packet can straddle a flush bucket, so fold every dogsketch of
		// an attempt before comparing.
		byAttempt := map[string]*sketchTotals{}
		for _, sk := range sketches {
			assert.Equal(c, hostname, sk.Host)
			key := attemptTag(sk.GetTags())
			if byAttempt[key] == nil {
				byAttempt[key] = &sketchTotals{min: math.Inf(1), max: math.Inf(-1)}
			}
			for _, d := range sk.Dogsketches {
				var bins int64
				for _, n := range d.N {
					bins += int64(n)
				}
				assert.Equal(c, d.Cnt, bins, "bin counts of %s must add up to its count", distMetric)
				byAttempt[key].add(d)
			}
		}

		var complete int
		for key, got := range byAttempt {
			assert.LessOrEqual(c, got.cnt, want.cnt, "%s %s has more samples than were sent", distMetric, key)
			if got.cnt == want.cnt {
				complete++
				assert.Equal(c, want, *got, "%s %s", distMetric, key)
			}
		}
		assert.NotZero(c, complete, "no complete %s sketch has reached the intake yet", distMetric)
	}, 5*time.Minute, 10*time.Second)
}

// nextAttemptTag returns a tag unique to one send, so each send gets its own context and its
// values are not merged with an earlier send that landed in the same flush bucket.
func (s *baselineSuite) nextAttemptTag() string {
	s.attempt++
	return fmt.Sprintf("%s%d", attemptTagPrefix, s.attempt)
}

// sendStatsd sends the given DogStatsD messages in a single UDP packet from the remote host.
func (s *baselineSuite) sendStatsd(c *assert.CollectT, messages ...string) {
	s.Env().RemoteHost.MustExecuteOn(c, fmt.Sprintf(`bash -c 'printf "%s" > /dev/udp/127.0.0.1/8125'`, strings.Join(messages, `\n`)))
}

func seriesHost(m *aggregator.MetricSeries) string {
	for _, r := range m.Resources {
		if r.Type == "host" {
			return r.Name
		}
	}
	return ""
}

func attemptTag(tags []string) string {
	for _, t := range tags {
		if strings.HasPrefix(t, attemptTagPrefix) {
			return t
		}
	}
	return ""
}

// sketchTotals is the summary of one or more dogsketches of the same context.
type sketchTotals struct {
	cnt      int64
	sum      float64
	min, max float64
}

func (t *sketchTotals) add(d metricspb.SketchPayload_Sketch_Dogsketch) {
	t.cnt += d.Cnt
	t.sum += d.Sum
	t.min = math.Min(t.min, d.Min)
	t.max = math.Max(t.max, d.Max)
}
