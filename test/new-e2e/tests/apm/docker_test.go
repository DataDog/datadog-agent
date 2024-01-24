// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apm

import (
	"flag"
	"net"
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
	"github.com/stretchr/testify/require"
)

var (
	devMode = flag.Bool("devmode", false, "enable dev mode")
)

type DockerFakeintakeSuite struct {
	e2e.BaseSuite[environments.DockerHost]
	transport transport
}

type transport int

const (
	UNDEFINED transport = iota
	UDS
	TCP
)

func suiteOpts(t *testing.T, opts ...awsdocker.ProvisionerOption) []e2e.SuiteOption {
	if !flag.Parsed() {
		flag.Parse()
	}

	isCI, _ := strconv.ParseBool(os.Getenv("CI"))
	if isCI {
		t.Skipf("blocked by APL-2786")
	}
	devModeEnv, _ := os.LookupEnv("E2E_DEVMODE")
	options := []e2e.SuiteOption{
		e2e.WithProvisioner(awsdocker.Provisioner(opts...)),
	}
	if devModeE, err := strconv.ParseBool(devModeEnv); (err == nil && devModeE) || *devMode {
		t.Log("Running in Dev Mode.")
		options = append(options, e2e.WithDevMode())
	}
	return options
}

// TestDockerFakeintakeSuiteTCP runs basic Trace Agent tests over the UDS transport
func TestDockerFakeintakeSuiteUDS(t *testing.T) {
	options := suiteOpts(t, awsdocker.WithAgentOptions(
		// Enable the UDS receiver in the trace-agent
		dockeragentparams.WithAgentServiceEnvVariable(
			"DD_APM_RECEIVER_SOCKET",
			pulumi.String("/var/run/datadog/apm.socket")),
		// Optional: UDS is more reliable for statsd metrics
		dockeragentparams.WithAgentServiceEnvVariable(
			"STATSD_URL",
			pulumi.String("unix:///var/run/datadog/dsd.socket")),
	))
	e2e.Run(t, &DockerFakeintakeSuite{transport: UDS}, options...)
}

// TestDockerFakeintakeSuiteTCP runs basic Trace Agent tests over the TCP transport
func TestDockerFakeintakeSuiteTCP(t *testing.T) {
	e2e.Run(t, &DockerFakeintakeSuite{transport: TCP}, suiteOpts(t)...)
}

func (s *DockerFakeintakeSuite) TestContainerTagging() {
	// TODO: Container tagging with cgroup v2 currently only works over UDS
	// We should update this to run over TCP as well once that is implemented.
	if s.transport == UDS {
		s.Run("TracesContainContainerTag", s.TracesContainContainerTag)
		s.Run("StatsContainContainerTag", s.StatsContainContainerTag)
	} else {
		s.T().Skip("Container Tagging with Cgroup v2 only works on UDS")
	}
}

func (s *DockerFakeintakeSuite) TestTraceAgentMetrics() {
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
		assert.GreaterOrEqual(c, len(metrics), len(expected))
		for _, m := range metrics {
			delete(expected, m)
			if len(expected) == 0 {
				s.T().Log("All expected metrics are found")
				return
			}
		}
		assert.Empty(c, expected)
	}, 2*time.Minute, 10*time.Second, "Failed finding datadog.trace_agent.* metrics")
}

func (s *DockerFakeintakeSuite) TracesContainContainerTag() {
	defer runTracegen(s.Env().Host, "tracegen-traces", tracegenCfg{transport: s.transport})()
	s.EventuallyWithTf(func(c *assert.CollectT) {
		traces, err := s.Env().FakeIntake.Client().GetTraces()
		assert.NoError(c, err)
		assert.NotEmpty(c, traces)
		assert.True(c, hasContainerTag(traces, "container_name:tracegen-uds"))
	}, 2*time.Minute, 10*time.Second, "Failed finding traces with container tags")
}

func (s *DockerFakeintakeSuite) StatsContainContainerTag() {
	defer runTracegen(s.Env().Host, "tracegen-stats", tracegenCfg{transport: s.transport})()
	s.EventuallyWithTf(func(c *assert.CollectT) {
		stats, err := s.Env().FakeIntake.Client().GetAPMStats()
		assert.NoError(c, err)
		assert.NotEmpty(c, stats)
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

func (s *DockerFakeintakeSuite) TestBasicTrace() {
	const testService = "tracegen"

	// Wait for agent to be live
	s.T().Log("Waiting for Trace Agent to be live.")
	s.Require().NoError(waitRemotePort(s, 8126))

	// Run Trace Generator
	s.T().Log("Starting Trace Generator.")
	shutdown := runTracegen(s.Env().Host, testService, tracegenCfg{transport: s.transport})
	defer shutdown()

	s.T().Log("Waiting for traces.")
	s.EventuallyWithTf(func(c *assert.CollectT) {
		traces, err := s.Env().FakeIntake.Client().GetTraces()
		require.NoError(c, err)
		require.NotEmpty(c, traces)

		trace := traces[0]
		require.NoError(c, err)
		assert.Equal(c, s.Env().Agent.Client.Hostname(), trace.HostName)
		assert.Equal(c, trace.Env, "none")
		require.NotEmpty(c, trace.TracerPayloads)

		tp := trace.TracerPayloads[0]
		assert.Equal(c, tp.LanguageName, "go")
		require.NotEmpty(c, tp.Chunks)
		require.NotEmpty(c, tp.Chunks[0].Spans)
		spans := tp.Chunks[0].Spans
		for _, sp := range spans {
			assert.Equal(c, sp.Service, testService)
			assert.Contains(c, sp.Name, "tracegen")
			assert.Contains(c, sp.Meta, "language")
			assert.Equal(c, sp.Meta["language"], "go")
			assert.Contains(c, sp.Metrics, "_sampling_priority_v1")
			if sp.ParentID == 0 {
				assert.Equal(c, sp.Metrics["_dd.top_level"], float64(1))
				assert.Equal(c, sp.Metrics["_top_level"], float64(1))
			}
		}

	}, 2*time.Minute, 10*time.Second, "Failed to find traces with basic properties")
}

func waitRemotePort(v *DockerFakeintakeSuite, port uint16) error {
	var (
		c   net.Conn
		err error
	)
	for i := 0; i < 10; i++ {
		v.T().Logf("Waiting for remote:%v", port)
		c, err = v.Env().Host.DialRemotePort(port)
		if err != nil {
			v.T().Logf("Failed to dial remote:%v: %s\n", port, err)
			time.Sleep(1 * time.Second)
		} else {
			v.T().Logf("Connected to remote:%v\n", port)
			defer c.Close()
			break
		}
	}
	return err
}
