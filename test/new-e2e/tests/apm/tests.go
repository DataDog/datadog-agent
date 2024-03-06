// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apm

import (
	"fmt"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"

	"github.com/stretchr/testify/assert"
)

func testBasicTraces(c *assert.CollectT, service string, intake *components.FakeIntake, agent agentclient.Agent) {
	traces, err := intake.Client().GetTraces()
	assert.NoError(c, err)
	if !assert.NotEmpty(c, traces) {
		return
	}
	trace := traces[0]
	assert.Equal(c, agent.Hostname(), trace.HostName)
	assert.Equal(c, "none", trace.Env)
	if !assert.NotEmpty(c, trace.TracerPayloads) {
		return
	}
	tp := trace.TracerPayloads[0]
	assert.Equal(c, "go", tp.LanguageName)
	if !assert.NotEmpty(c, tp.Chunks) {
		return
	}
	if !assert.NotEmpty(c, tp.Chunks[0].Spans) {
		return
	}
	spans := tp.Chunks[0].Spans
	for _, sp := range spans {
		assert.Equal(c, service, sp.Service)
		assert.Contains(c, sp.Name, "tracegen")
		assert.Contains(c, sp.Meta, "language")
		assert.Equal(c, "go", sp.Meta["language"])
		assert.Contains(c, sp.Metrics, "_sampling_priority_v1")
		if sp.ParentID == 0 {
			assert.Equal(c, float64(1), sp.Metrics["_dd.top_level"])
			assert.Equal(c, float64(1), sp.Metrics["_top_level"])
		}
	}
}

func testStatsForService(t *testing.T, c *assert.CollectT, service string, intake *components.FakeIntake) {
	t.Helper()
	stats, err := intake.Client().GetAPMStats()
	assert.NoError(c, err)
	assert.NotEmpty(c, stats)
	t.Log("Got apm stats", stats)
	assert.True(c, hasStatsForService(stats, service))
}

func testTracesHaveContainerTag(t *testing.T, c *assert.CollectT, service string, intake *components.FakeIntake) {
	t.Helper()
	traces, err := intake.Client().GetTraces()
	assert.NoError(c, err)
	assert.NotEmpty(c, traces)
	t.Log("Got traces", traces)
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
	t.Helper()
	expected := map[string]struct{}{
		// "datadog.trace_agent.started":                         {}, // FIXME: this metric is flaky
		"datadog.trace_agent.heartbeat":                        {},
		"datadog.trace_agent.heap_alloc":                       {},
		"datadog.trace_agent.cpu_percent":                      {},
		"datadog.trace_agent.events.max_eps.current_rate":      {},
		"datadog.trace_agent.events.max_eps.max_rate":          {},
		"datadog.trace_agent.events.max_eps.reached_max":       {},
		"datadog.trace_agent.events.max_eps.sample_rate":       {},
		"datadog.trace_agent.sampler.kept":                     {},
		"datadog.trace_agent.sampler.rare.hits":                {},
		"datadog.trace_agent.sampler.rare.misses":              {},
		"datadog.trace_agent.sampler.rare.shrinks":             {},
		"datadog.trace_agent.sampler.seen":                     {},
		"datadog.trace_agent.sampler.size":                     {},
		"datadog.trace_agent.stats_writer.bytes":               {},
		"datadog.trace_agent.stats_writer.client_payloads":     {},
		"datadog.trace_agent.stats_writer.encode_ms.avg":       {},
		"datadog.trace_agent.stats_writer.encode_ms.count":     {},
		"datadog.trace_agent.stats_writer.encode_ms.max":       {},
		"datadog.trace_agent.stats_writer.errors":              {},
		"datadog.trace_agent.stats_writer.payloads":            {},
		"datadog.trace_agent.stats_writer.retries":             {},
		"datadog.trace_agent.stats_writer.splits":              {},
		"datadog.trace_agent.stats_writer.stats_buckets":       {},
		"datadog.trace_agent.stats_writer.stats_entries":       {},
		"datadog.trace_agent.trace_writer.bytes":               {},
		"datadog.trace_agent.trace_writer.bytes_uncompressed":  {},
		"datadog.trace_agent.trace_writer.errors":              {},
		"datadog.trace_agent.trace_writer.events":              {},
		"datadog.trace_agent.trace_writer.payloads":            {},
		"datadog.trace_agent.trace_writer.retries":             {},
		"datadog.trace_agent.trace_writer.spans":               {},
		"datadog.trace_agent.trace_writer.traces":              {},
		"datadog.trace_agent.trace_writer.encode_ms.avg":       {},
		"datadog.trace_agent.trace_writer.encode_ms.count":     {},
		"datadog.trace_agent.trace_writer.encode_ms.max":       {},
		"datadog.trace_agent.receiver.config_process_ms.avg":   {},
		"datadog.trace_agent.receiver.config_process_ms.count": {},
		"datadog.trace_agent.receiver.config_process_ms.max":   {},
	}
	metrics, err := intake.Client().GetMetricNames()
	assert.NoError(c, err)
	t.Log("Got metric names", metrics)
	assert.GreaterOrEqual(c, len(metrics), len(expected))
	for _, m := range metrics {
		delete(expected, m)
		if len(expected) == 0 {
			t.Log("All expected metrics are found")
			return
		}
	}
	t.Log("Remaining metrics", expected)
	assert.Empty(c, expected)
}

func testTraceAgentMetricTags(t *testing.T, c *assert.CollectT, service string, intake *components.FakeIntake) {
	t.Helper()
	expected := map[string]struct{}{
		"datadog.trace_agent.receiver.payload_accepted":         {},
		"datadog.trace_agent.receiver.trace":                    {},
		"datadog.trace_agent.receiver.traces_received":          {},
		"datadog.trace_agent.receiver.spans_received":           {},
		"datadog.trace_agent.receiver.traces_bytes":             {},
		"datadog.trace_agent.receiver.traces_filtered":          {},
		"datadog.trace_agent.receiver.spans_dropped":            {},
		"datadog.trace_agent.receiver.spans_filtered":           {},
		"datadog.trace_agent.receiver.traces_priority":          {},
		"datadog.trace_agent.normalizer.traces_dropped":         {},
		"datadog.trace_agent.normalizer.spans_malformed":        {},
		"datadog.trace_agent.receiver.client_dropped_p0_spans":  {},
		"datadog.trace_agent.receiver.client_dropped_p0_traces": {},
		"datadog.trace_agent.receiver.events_sampled":           {},
		"datadog.trace_agent.receiver.events_extracted":         {},
	}
	serviceTag := "service:" + service
	for m := range expected {
		filtered, err := intake.Client().FilterMetrics(m, fakeintake.WithTags[*aggregator.MetricSeries]([]string{serviceTag}))
		if assert.NoError(c, err) && assert.NotEmpty(c, filtered) {
			delete(expected, m)
		}
	}
	assert.Empty(c, expected)
}
