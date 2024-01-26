package apm

import (
	"fmt"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testBasicTraces(c *assert.CollectT, service string, intake *components.FakeIntake, agent agentclient.Agent) {
	traces, err := intake.Client().GetTraces()
	require.NoError(c, err)
	require.NotEmpty(c, traces)

	trace := traces[0]
	require.NoError(c, err)
	assert.Equal(c, agent.Hostname(), trace.HostName)
	assert.Equal(c, trace.Env, "none")
	require.NotEmpty(c, trace.TracerPayloads)

	tp := trace.TracerPayloads[0]
	assert.Equal(c, tp.LanguageName, "go")
	require.NotEmpty(c, tp.Chunks)
	require.NotEmpty(c, tp.Chunks[0].Spans)
	spans := tp.Chunks[0].Spans
	for _, sp := range spans {
		assert.Equal(c, sp.Service, service)
		assert.Contains(c, sp.Name, "tracegen")
		assert.Contains(c, sp.Meta, "language")
		assert.Equal(c, sp.Meta["language"], "go")
		assert.Contains(c, sp.Metrics, "_sampling_priority_v1")
		if sp.ParentID == 0 {
			assert.Equal(c, sp.Metrics["_dd.top_level"], float64(1))
			assert.Equal(c, sp.Metrics["_top_level"], float64(1))
		}
	}
}

func testStatsForService(c *assert.CollectT, service string, intake *components.FakeIntake) {
	stats, err := intake.Client().GetAPMStats()
	assert.NoError(c, err)
	assert.NotEmpty(c, stats)
	assert.True(c, hasStatsForService(stats, service))
}

func testTracesHaveContainerTag(c *assert.CollectT, service string, intake *components.FakeIntake) {
	traces, err := intake.Client().GetTraces()
	assert.NoError(c, err)
	assert.NotEmpty(c, traces)
	assert.True(c, hasContainerTag(traces, fmt.Sprintf("container_name:%s", service)))
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

func testTraceAgentMetrics(t *testing.T, c *assert.CollectT, intake *components.FakeIntake) {
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
	metrics, err := intake.Client().GetMetricNames()
	assert.NoError(c, err)
	assert.GreaterOrEqual(c, len(metrics), len(expected))
	for _, m := range metrics {
		delete(expected, m)
		if len(expected) == 0 {
			t.Log("All expected metrics are found")
			return
		}
	}
	assert.Empty(c, expected)
}
