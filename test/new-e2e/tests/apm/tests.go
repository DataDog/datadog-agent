// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apm

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

// The convert-traces feature is enabled by default, so the agent now serializes
// tracer payloads in the v1 string-indexed idx format (AgentPayload.IdxTracerPayloads).
// Instead of the legacy tracerPayloads field, span/chunk/payload metadata is
// carried as references into a per-payload string table, and tags/meta/metrics
// live in a single attributes map keyed by string reference. The assertions below
// resolve those references with the fakeintake.Idx* accessors.

// clientStatsHasService reports whether the client stats payload contains stats
// for the given service.
func clientStatsHasService(s *trace.ClientStatsPayload, service string) bool {
	if s.Service == service {
		return true
	}
	for _, bucket := range s.Stats {
		for _, gs := range bucket.Stats {
			if gs.Service == service {
				return true
			}
		}
	}
	return false
}

func testBasicTraces(c *assert.CollectT, service string, intake *components.FakeIntake, agent agentclient.Agent) {
	traces, err := intake.Client().GetTraces()
	assert.NoError(c, err)
	if !assert.NotEmpty(c, traces) {
		return
	}
	// The fakeintake accumulates every payload it receives, and a leaked
	// environment whose agent keeps posting to a recycled intake IP can inject
	// unrelated payloads (e.g. another suite's traces). Select the payload for
	// our own tracegen service instead of blindly taking traces[0].
	var tp *idx.TracerPayload
	var hostName, env string
	for _, tr := range traces {
		for _, p := range tr.IdxTracerPayloads {
			if fakeintake.IdxTracerPayloadHasService(p, service) {
				tp, hostName, env = p, tr.HostName, tr.Env
				break
			}
		}
		if tp != nil {
			break
		}
	}
	if !assert.NotNil(c, tp, "no trace payload found for service %s", service) {
		return
	}
	strs := tp.Strings
	assert.Equal(c, agent.Hostname(), hostName)
	assert.Equal(c, "none", env)
	assert.Equal(c, "go", fakeintake.IdxStr(strs, tp.LanguageNameRef))
	assert.False(c, fakeintake.IdxHasAttr(strs, tp.Attributes, "_dd.apm_mode"))
	if !assert.NotEmpty(c, tp.Chunks) {
		return
	}
	if !assert.NotEmpty(c, tp.Chunks[0].Spans) {
		return
	}
	spans := tp.Chunks[0].Spans
	for _, sp := range spans {
		assert.Equal(c, service, fakeintake.IdxStr(strs, sp.ServiceRef))
		assert.Contains(c, fakeintake.IdxStr(strs, sp.NameRef), "tracegen")
		language, ok := fakeintake.IdxStrAttr(strs, sp.Attributes, "language")
		assert.True(c, ok, "span missing language attribute")
		assert.Equal(c, "go", language)
		assert.True(c, fakeintake.IdxHasAttr(strs, sp.Attributes, "_sampling_priority_v1"))
		if sp.ParentID == 0 {
			topLevel, _ := fakeintake.IdxNumAttr(strs, sp.Attributes, "_dd.top_level")
			assert.Equal(c, float64(1), topLevel)
			legacyTopLevel, _ := fakeintake.IdxNumAttr(strs, sp.Attributes, "_top_level")
			assert.Equal(c, float64(1), legacyTopLevel)
		}
	}
}

func testTPS(c *assert.CollectT, intake *components.FakeIntake, service string, tps float64) {
	traces, err := intake.Client().GetTraces()
	assert.NoError(c, err)
	if !assert.NotEmpty(c, traces) {
		return
	}

	// Only assert on payloads produced by our own tracegen service; the intake
	// may hold unrelated payloads with a different TargetTPS.
	found := false
	for _, p := range traces {
		if !fakeintake.IdxPayloadHasService(p, service) {
			continue
		}
		found = true
		assert.Equal(c, tps, p.TargetTPS, "invalid TargetTPS found in traces")
	}
	assert.True(c, found, "no trace payload found for service %s", service)
}

func testStatsForService(t *testing.T, c *assert.CollectT, service string, expectedPeerTag string, intake *components.FakeIntake) {
	t.Helper()
	stats, err := intake.Client().GetAPMStats()
	assert.NoError(c, err)
	assert.NotEmpty(c, stats)
	t.Logf("Got %d apm stats", len(stats))
	assert.True(c, hasStatsForService(stats, service), "got stats: %v", stats)
	if expectedPeerTag != "" {
		assert.True(c, hasPeerTagsStats(stats, expectedPeerTag), "got stats: %v", stats)
	}
}

func testTracesHaveContainerTag(t *testing.T, c *assert.CollectT, service string, intake *components.FakeIntake) {
	t.Helper()
	traces, err := intake.Client().GetTraces()
	assert.NoError(c, err)
	assert.NotEmpty(c, traces)
	t.Logf("Got %d apm traces", len(traces))
	assert.True(c, hasContainerTag(traces, "container_name:"+service), "got traces: %v", traces)
}

func testProcessTraces(c *assert.CollectT, intake *components.FakeIntake, processTags string) {
	traces, err := intake.Client().GetTraces()
	assert.NoError(c, err)
	if !assert.NotEmpty(c, traces) {
		return
	}
	// Fakeintake accumulates across tests; old payloads (without _dd.tags.process) may
	// arrive after the flush due to agent pipeline buffering. Assert that at least one
	// TracerPayload carries the expected process
	found := false
	for _, p := range traces {
		for _, tp := range p.IdxTracerPayloads {
			tags, ok := fakeintake.IdxStrAttr(tp.Strings, tp.Attributes, "_dd.tags.process")
			if !ok {
				continue
			}
			assert.Equal(c, processTags, tags)
			found = true
		}
	}
	assert.True(c, found, "no TracerPayload had _dd.tags.process=%q", processTags)
}

func testStatsHaveProcessTags(c *assert.CollectT, intake *components.FakeIntake, processTags string) {
	stats, err := intake.Client().GetAPMStats()
	assert.NoError(c, err)
	if !assert.NotEmpty(c, stats) {
		return
	}
	// Fakeintake accumulates across tests; old payloads (without _dd.tags.process) may
	// arrive after the flush due to agent pipeline buffering. Assert that at least one
	// TracerPayload carries the expected process
	found := false
	for _, p := range stats {
		for _, s := range p.StatsPayload.Stats {
			if s.ProcessTags == "" {
				continue
			}
			assert.Equal(c, processTags, s.ProcessTags)
			found = true
		}
	}
	assert.True(c, found, "no stats payload had ProcessTags=%q", processTags)
}

func testStatsHaveContainerTags(t *testing.T, c *assert.CollectT, service string, intake *components.FakeIntake) {
	t.Helper()
	stats, err := intake.Client().GetAPMStats()
	assert.NoError(c, err)
	assert.NotEmpty(c, stats)
	t.Logf("Got %d apm stats", len(stats))

	for _, p := range stats {
		for _, s := range p.StatsPayload.Stats {
			for _, bucket := range s.Stats {
				for _, ss := range bucket.Stats {
					if ss.Service == service {
						assert.NotEmpty(c, s.ContainerID, "ContainerID should not be empty. Got Stats: %v", stats)
						assert.NotEmpty(c, s.Tags, "Container Tags should not be empty. Got Stats: %v", stats)
						assert.Contains(c, s.Tags, "container_name:"+service)
					}
				}
			}
		}
	}
}

func testAutoVersionTraces(t *testing.T, c *assert.CollectT, service string, intake *components.FakeIntake) {
	t.Helper()
	traces, err := intake.Client().GetTraces()
	assert.NoError(c, err)
	assert.NotEmpty(c, traces)
	t.Logf("Got %d apm traces", len(traces))
	found := false
	for _, tr := range traces {
		for _, tp := range tr.IdxTracerPayloads {
			if !fakeintake.IdxTracerPayloadHasService(tp, service) {
				continue
			}
			found = true
			containerTags, _ := fakeintake.IdxStrAttr(tp.Strings, tp.Attributes, "_dd.tags.container")
			t.Log("Tracer Payload Tags:", containerTags)
			ctags, ok := getContainerTags(t, tp)
			assert.True(c, ok, "expected to find container tags at _dd.tags.container")
			imageTag, ok := ctags["image_tag"]
			assert.True(c, ok, "expected to find image_tag in container tags")
			t.Logf("Got image Tag: %v", imageTag)
			assert.Equal(c, apps.Version, imageTag)
		}
	}
	assert.True(c, found, "no trace payload found for service %s", service)
}

func tracesSampledByProbabilitySampler(t *testing.T, c *assert.CollectT, service string, intake *components.FakeIntake) {
	t.Helper()
	traces, err := intake.Client().GetTraces()
	assert.NoError(c, err)
	assert.NotEmpty(c, traces)
	t.Logf("Got %d apm traces", len(traces))
	// In the v1 idx format the decision maker (_dd.p.dm) is no longer carried as a
	// chunk attribute: it is promoted to the dedicated SamplingMechanism chunk field.
	// The probabilistic sampler sets mechanism 9 (the legacy "_dd.p.dm: -9" tag).
	const probabilitySamplingMechanism = 9
	found := false
	for _, p := range traces {
		for _, tp := range p.IdxTracerPayloads {
			if !fakeintake.IdxTracerPayloadHasService(tp, service) {
				continue
			}
			for _, chunk := range tp.Chunks {
				found = true
				if chunk.SamplingMechanism != probabilitySamplingMechanism {
					t.Errorf("Expected chunk SamplingMechanism == %d, but got %d for service %s", probabilitySamplingMechanism, chunk.SamplingMechanism, fakeintake.IdxStr(tp.Strings, chunk.Spans[0].ServiceRef))
				}
			}
		}
	}
	assert.True(c, found, "no trace chunks found for service %s", service)
}

func testAutoVersionStats(t *testing.T, c *assert.CollectT, service string, intake *components.FakeIntake) {
	t.Helper()
	stats, err := intake.Client().GetAPMStats()
	assert.NoError(c, err)
	assert.NotEmpty(c, stats)
	t.Logf("Got %d apm stats", len(stats))
	found := false
	for _, p := range stats {
		for _, s := range p.StatsPayload.Stats {
			if !clientStatsHasService(s, service) {
				continue
			}
			found = true
			t.Log("Client Payload:", spew.Sdump(s))
			t.Logf("Got image Tag: %v", s.GetImageTag())
			assert.Equal(c, apps.Version, s.GetImageTag())
			t.Logf("Got git commit sha: %v", s.GetGitCommitSha())
			assert.Equal(c, "abcd1234", s.GetGitCommitSha())
		}
	}
	assert.True(c, found, "no stats payload found for service %s", service)
}

func testIsTraceRootTag(t *testing.T, c *assert.CollectT, service string, intake *components.FakeIntake) {
	t.Helper()
	stats, err := intake.Client().GetAPMStats()
	assert.NoError(c, err)
	assert.NotEmpty(c, stats)
	t.Logf("Got %d apm stats", len(stats))
	found := false
	for _, p := range stats {
		for _, s := range p.StatsPayload.Stats {
			t.Log("Client Payload:", spew.Sdump(s))
			for _, b := range s.Stats {
				for _, cs := range b.Stats {
					if cs.Service != service {
						continue
					}
					found = true
					t.Logf("Got IsTraceRoot: %v", cs.GetIsTraceRoot())
					assert.Equal(c, trace.Trilean_TRUE, cs.GetIsTraceRoot())
				}
			}
		}
	}
	assert.True(c, found, "no stats found for service %s", service)
}

func getContainerTags(t *testing.T, tp *idx.TracerPayload) (map[string]string, bool) {
	ctags, ok := fakeintake.IdxStrAttr(tp.Strings, tp.Attributes, "_dd.tags.container")
	if !ok {
		return nil, false
	}
	splits := strings.Split(ctags, ",")
	m := make(map[string]string)
	for _, s := range splits {
		kv := strings.SplitN(s, ":", 2)
		if !assert.Len(t, kv, 2, "malformed container tag: %v", s) {
			continue
		}
		m[kv[0]] = kv[1]
	}
	return m, true
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

func hasPeerTagsStats(payloads []*aggregator.APMStatsPayload, fullTag string) bool {
	for _, p := range payloads {
		for _, s := range p.StatsPayload.Stats {
			for _, bucket := range s.Stats {
				for _, ss := range bucket.Stats {
					if slices.Contains(ss.GetPeerTags(), fullTag) {
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
		for _, tp := range p.IdxTracerPayloads {
			tags, ok := fakeintake.IdxStrAttr(tp.Strings, tp.Attributes, "_dd.tags.container")
			if ok && strings.Count(tags, tag) > 0 {
				return true
			}
		}
	}
	return false
}

func testTraceAgentMetrics(t *testing.T, c *assert.CollectT, intake *components.FakeIntake, rcEnabled bool) {
	t.Helper()
	expected := map[string]struct{}{
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
		"datadog.trace_agent.trace_writer.encode_ms.avg":      {},
		"datadog.trace_agent.trace_writer.encode_ms.count":    {},
		"datadog.trace_agent.trace_writer.encode_ms.max":      {},
	}
	if rcEnabled {
		expected["datadog.trace_agent.receiver.config_process_ms.avg"] = struct{}{}
		expected["datadog.trace_agent.receiver.config_process_ms.count"] = struct{}{}
		expected["datadog.trace_agent.receiver.config_process_ms.max"] = struct{}{}
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
		"datadog.trace_agent.receiver.payload_accepted": {},
		"datadog.trace_agent.receiver.trace":            {},
		"datadog.trace_agent.receiver.traces_received":  {},
		"datadog.trace_agent.receiver.spans_received":   {},
		"datadog.trace_agent.receiver.traces_bytes":     {},
		"datadog.trace_agent.receiver.spans_dropped":    {},
		"datadog.trace_agent.receiver.traces_priority":  {},
		// These metrics are only emitted when non-zero to reduce cardinality
		// "datadog.trace_agent.normalizer.traces_dropped":         {},
		// "datadog.trace_agent.normalizer.spans_malformed":        {},
		// "datadog.trace_agent.receiver.client_dropped_p0_spans":  {},
		// "datadog.trace_agent.receiver.client_dropped_p0_traces": {},
		// "datadog.trace_agent.receiver.events_sampled":   {},
		// "datadog.trace_agent.receiver.events_extracted":         {},
		// "datadog.trace_agent.receiver.traces_filtered":  {},
		// "datadog.trace_agent.receiver.spans_filtered":  {},

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

func hasStatsForResource(payloads []*aggregator.APMStatsPayload, resource string) bool {
	for _, p := range payloads {
		for _, s := range p.StatsPayload.Stats {
			for _, bucket := range s.Stats {
				for _, ss := range bucket.Stats {
					if ss.Resource == resource {
						return true
					}
				}
			}
		}
	}
	return false
}

func hasTraceForResource(payloads []*aggregator.TracePayload, resource string) bool {
	for _, p := range payloads {
		for _, tp := range p.IdxTracerPayloads {
			for _, c := range tp.Chunks {
				for _, s := range c.Spans {
					if fakeintake.IdxStr(tp.Strings, s.ResourceRef) == resource {
						return true
					}
				}
			}
		}
	}
	return false
}

func waitTracegenShutdown(s *suite.Suite, intake *components.FakeIntake) {
	// TODO(knusbaum): we can ideally assert the poison pill eventually arrives,
	// but currently it seems it does not always.
	//s.EventuallyWithTf(func(c *assert.CollectT) {
	//	hasPoisonPill(s.T(), c, intake)
	//}, 20*time.Second, 1*time.Second, "Failed to find poison pill from tracegen shutdown.")

	s.T().Helper()
	begin := time.Now()
	max := begin.Add(20 * time.Second)
	for {
		if hasPoisonPill(s.T(), intake) {
			// success
			return
		}
		if time.Now().After(max) {
			// Timeout, continue tests assuming
			// it's long enough that the pipeline is clear.
			return
		}
		time.Sleep(1 * time.Second)
	}
}

func hasPoisonPill(t *testing.T, intake *components.FakeIntake) bool {
	t.Helper()
	stats, err := intake.Client().GetAPMStats()
	assert.NoError(t, err)
	t.Logf("Got %d stats", len(stats))
	if !hasStatsForResource(stats, "poison_pill") { // tracegen sends this resource as the last trace before shutting down.
		return false
	}
	traces, err := intake.Client().GetTraces()
	assert.NoError(t, err)
	t.Logf("Got %d traces", len(traces))
	return hasTraceForResource(traces, "poison_pill")
}

func testAPMMode(c *assert.CollectT, intake *components.FakeIntake, service string, expectedAPMMode string) {
	traces, err := intake.Client().GetTraces()
	assert.NoError(c, err)
	if !assert.NotEmpty(c, traces) {
		return
	}
	found := false
	for _, p := range traces {
		if !fakeintake.IdxPayloadHasService(p, service) {
			continue
		}
		found = true
		if expectedAPMMode == "" {
			// assert that apm mode tag does not exist
			v, ok := p.Tags["_dd.apm_mode"]
			assert.False(c, ok)
			assert.Empty(c, v)
			continue
		}
		assert.Equal(c, expectedAPMMode, p.Tags["_dd.apm_mode"])
	}
	assert.True(c, found, "no trace payload found for service %s", service)
}
