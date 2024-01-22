// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apm

import (
	"os"
	"strconv"
	"testing"
	"time"

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
	envs := pulumi.StringMap{"STATSD_URL": pulumi.String("unix:///var/run/datadog/dsd.socket")} // UDP appears to be flaky, use UDS instead.
	options := []e2e.SuiteOption{
		e2e.WithProvisioner(awsdocker.Provisioner(awsdocker.WithAgentOptions(dockeragentparams.WithEnvironmentVariables(envs)))),
	}
	if devMode, err := strconv.ParseBool(devModeEnv); err == nil && devMode {
		options = append(options, e2e.WithDevMode())
	}
	e2e.Run(t, &DockerFakeintakeSuite{}, options...)
}

func (s *DockerFakeintakeSuite) TestAPMDocker() {
	s.Run("TraceAgentMetrics", s.TraceAgentMetrics)
}

func (s *DockerFakeintakeSuite) TraceAgentMetrics() {
	s.EventuallyWithTf(func(c *assert.CollectT) {
		expected := map[string]struct{}{
			"datadog.trace_agent.heartbeat":                       {},
			"datadog.trace_agent.started":                         {},
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
	}, 5*time.Minute, 10*time.Second, "Failed finding datadog.trace_agent.* metrics")
}
