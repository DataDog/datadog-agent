// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apm

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awsdocker "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/docker"
	"github.com/DataDog/test-infra-definitions/components/datadog/dockeragentparams"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
)

type DockerFakeintakeSuite struct {
	e2e.BaseSuite[environments.DockerHost]
}

func TestDockerFakeintakeSuite(t *testing.T) {
	devModeEnv, _ := os.LookupEnv("E2E_DEVMODE")
	options := []e2e.SuiteOption{
		e2e.WithProvisioner(awsdocker.Provisioner(awsdocker.WithAgentOptions(
			dockeragentparams.WithAgentServiceEnvVariable("DD_APM_RECEIVER_SOCKET", pulumi.String("/var/run/datadog/apm.socket")), // Enable the UDS receiver in the trace-agent
			dockeragentparams.WithAgentServiceEnvVariable("STATSD_URL", pulumi.String("unix:///var/run/datadog/dsd.socket")),      // Optional: UDS is more reliable for statsd metrics
		))),
	}
	if devMode, err := strconv.ParseBool(devModeEnv); err == nil && devMode {
		options = append(options, e2e.WithDevMode())
	}
	e2e.Run(t, &DockerFakeintakeSuite{}, options...)
}

func (s *DockerFakeintakeSuite) TestAPMDocker() {
	s.Run("TraceAgentMetrics", s.TraceAgentMetrics)
	s.Run("TracesOnUDS", s.TracesOnUDS)
	s.Run("StatsOnUDS", s.StatsOnUDS)
}

func (s *DockerFakeintakeSuite) TraceAgentMetrics() {
	s.EventuallyWithTf(func(c *assert.CollectT) {
		expected := map[string]struct{}{
			// "datadog.trace_agent.started":                         {}, // FIXME: this metric is flaky
			"datadog.trace_agent.heartbeat":                       {},
			"datadog.trace_agent.heap_alloc":                      {},
			"datadog.trace_agent.cpu_percent":                     {},
			"datadog.trace_agent.events.max_eps.current_rate":     {},
			"datadog.trace_agent.events.max_eps.max_rate":         {},
			"datadog.trace_agent.events.max_eps.reached_max":      {},
			"datadog.trace_agent.events.max_eps.sample_rate":      {},
			"datadog.trace_agent.sampler.kept":                    {},
			"datadog.trace_agent.sampler.rare.hits":               {},
			"datadog.trace_agent.sampler.rare.misses":             {},
			"datadog.trace_agent.sampler.rare.shrinks":            {},
			"datadog.trace_agent.sampler.seen":                    {},
			"datadog.trace_agent.sampler.size":                    {},
			"datadog.trace_agent.stats_writer.bytes":              {},
			"datadog.trace_agent.stats_writer.client_payloads":    {},
			"datadog.trace_agent.stats_writer.encode_ms.avg":      {},
			"datadog.trace_agent.stats_writer.encode_ms.count":    {},
			"datadog.trace_agent.stats_writer.encode_ms.max":      {},
			"datadog.trace_agent.stats_writer.errors":             {},
			"datadog.trace_agent.stats_writer.payloads":           {},
			"datadog.trace_agent.stats_writer.retries":            {},
			"datadog.trace_agent.stats_writer.splits":             {},
			"datadog.trace_agent.stats_writer.stats_buckets":      {},
			"datadog.trace_agent.stats_writer.stats_entries":      {},
			"datadog.trace_agent.trace_writer.bytes":              {},
			"datadog.trace_agent.trace_writer.bytes_uncompressed": {},
			"datadog.trace_agent.trace_writer.errors":             {},
			"datadog.trace_agent.trace_writer.events":             {},
			"datadog.trace_agent.trace_writer.payloads":           {},
			"datadog.trace_agent.trace_writer.retries":            {},
			"datadog.trace_agent.trace_writer.spans":              {},
			"datadog.trace_agent.trace_writer.traces":             {},
		}
		metrics, err := s.Env().FakeIntake.Client().GetMetricNames()
		assert.NoError(c, err)
		s.T().Log("Got metric names", metrics)
		assert.GreaterOrEqual(c, len(metrics), len(expected))
		for _, m := range metrics {
			delete(expected, m)
			if len(expected) == 0 {
				s.T().Log("All expected metrics are found")
				return
			}
		}
		s.T().Log("Remaining metrics", expected)
		assert.Empty(c, expected)
	}, 2*time.Minute, 10*time.Second, "Failed finding datadog.trace_agent.* metrics")
}

func (s *DockerFakeintakeSuite) TracesOnUDS() {
	run, rm := dockerRunUDS("tracegen-uds")
	s.Env().Host.MustExecute(run)
	defer s.Env().Host.MustExecute(rm)
	s.EventuallyWithTf(func(c *assert.CollectT) {
		traces, err := s.Env().FakeIntake.Client().GetTraces()
		assert.NoError(c, err)
		assert.NotEmpty(c, traces)
		s.T().Log("Got traces", traces)
		assert.True(c, hasContainerTag(traces, "container_name:tracegen-uds"))
	}, 2*time.Minute, 10*time.Second, "Failed finding traces with container tags")
}

func (s *DockerFakeintakeSuite) StatsOnUDS() {
	run, rm := dockerRunUDS("tracegen-stats-uds")
	s.Env().Host.MustExecute(run)
	defer s.Env().Host.MustExecute(rm)
	s.EventuallyWithTf(func(c *assert.CollectT) {
		stats, err := s.Env().FakeIntake.Client().GetAPMStats()
		assert.NoError(c, err)
		assert.NotEmpty(c, stats)
		s.T().Log("Got apm stats", stats)
		assert.True(c, hasStatsForService(stats, "tracegen-stats-uds"))
	}, 2*time.Minute, 10*time.Second, "Failed finding stats")
}

func hasStatsForService(payloads []*aggregator.APMStatsPayload, service string) bool {
	for _, p := range payloads {
		for _, s := range p.StatsPayload.Stats {
			for _, bucket := range s.Stats {
				for _, ss := range bucket.Stats {
					if ss.Service == service {
						return true
					}
				}
			}
		}
	}
	return false
}

func hasContainerTag(payloads []*aggregator.TracePayload, tag string) bool {
	for _, p := range payloads {
		for _, t := range p.AgentPayload.TracerPayloads {
			tags, ok := t.Tags["_dd.tags.container"]
			if ok && strings.Count(tags, tag) > 0 {
				return true
			}
		}
	}
	return false
}

func dockerRunUDS(service string) (string, string) {
	// TODO: use a proper docker-compose defintion for tracegen
	run := "docker run -d --network host --rm --name " + service + " -v /var/run/datadog/:/var/run/datadog/ -e DD_TRACE_AGENT_URL=unix:///var/run/datadog/apm.socket -e DD_SERVICE=" + service + " ghcr.io/datadog/apps-tracegen:main"
	rm := "docker rm -f " + service
	return run, rm
}
