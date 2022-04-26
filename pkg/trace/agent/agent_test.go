// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"bytes"
	"context"
	"io/ioutil"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/obfuscate"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/event"
	"github.com/DataDog/datadog-agent/pkg/trace/filters"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/trace/stats"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
	"github.com/DataDog/datadog-agent/pkg/trace/writer"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test to make sure that the joined effort of the quantizer and truncator, in that order, produce the
// desired string
func TestFormatTrace(t *testing.T) {
	assert := assert.New(t)
	resource := "SELECT name FROM people WHERE age = 42"
	rep := strings.Repeat(" AND age = 42", 25000)
	resource = resource + rep
	testTrace := pb.Trace{
		&pb.Span{
			Resource: resource,
			Type:     "sql",
		},
	}
	result := formatTrace(testTrace)[0]

	assert.Equal(5000, len(result.Resource))
	assert.NotEqual("Non-parsable SQL query", result.Resource)
	assert.NotContains(result.Resource, "42")
	assert.Contains(result.Resource, "SELECT name FROM people WHERE age = ?")

	assert.Equal(25003, len(result.Meta["sql.query"])) // Ellipsis added in quantizer
	assert.NotEqual("Non-parsable SQL query", result.Meta["sql.query"])
	assert.NotContains(result.Meta["sql.query"], "42")
	assert.Contains(result.Meta["sql.query"], "SELECT name FROM people WHERE age = ?")
}

func TestProcess(t *testing.T) {
	t.Run("Replacer", func(t *testing.T) {
		// Ensures that for "sql" type spans:
		// • obfuscator runs before replacer
		// • obfuscator obfuscates both resource and "sql.query" tag
		// • resulting resource is obfuscated with replacements applied
		// • resulting "sql.query" tag is obfuscated with no replacements applied
		cfg := config.New()
		cfg.Endpoints[0].APIKey = "test"
		cfg.ReplaceTags = []*config.ReplaceRule{{
			Name: "resource.name",
			Re:   regexp.MustCompile("AND.*"),
			Repl: "...",
		}}
		ctx, cancel := context.WithCancel(context.Background())
		agnt := NewAgent(ctx, cfg)
		defer cancel()

		now := time.Now()
		span := &pb.Span{
			TraceID:  1,
			SpanID:   1,
			Resource: "SELECT name FROM people WHERE age = 42 AND extra = 55",
			Type:     "sql",
			Start:    now.Add(-time.Second).UnixNano(),
			Duration: (500 * time.Millisecond).Nanoseconds(),
		}

		agnt.Process(&api.Payload{
			TracerPayload: testutil.TracerPayloadWithChunk(testutil.TraceChunkWithSpan(span)),
			Source:        info.NewReceiverStats().GetTagStats(info.Tags{}),
		})

		assert := assert.New(t)
		assert.Equal("SELECT name FROM people WHERE age = ? ...", span.Resource)
		assert.Equal("SELECT name FROM people WHERE age = ? AND extra = ?", span.Meta["sql.query"])
	})

	t.Run("Blacklister", func(t *testing.T) {
		cfg := config.New()
		cfg.Endpoints[0].APIKey = "test"
		cfg.Ignore["resource"] = []string{"^INSERT.*"}
		ctx, cancel := context.WithCancel(context.Background())
		agnt := NewAgent(ctx, cfg)
		defer cancel()

		now := time.Now()
		spanValid := &pb.Span{
			TraceID:  1,
			SpanID:   1,
			Resource: "SELECT name FROM people WHERE age = 42 AND extra = 55",
			Type:     "sql",
			Start:    now.Add(-time.Second).UnixNano(),
			Duration: (500 * time.Millisecond).Nanoseconds(),
		}
		spanInvalid := &pb.Span{
			TraceID:  1,
			SpanID:   1,
			Resource: "INSERT INTO db VALUES (1, 2, 3)",
			Type:     "sql",
			Start:    now.Add(-time.Second).UnixNano(),
			Duration: (500 * time.Millisecond).Nanoseconds(),
		}

		want := agnt.Receiver.Stats.GetTagStats(info.Tags{})
		assert := assert.New(t)

		agnt.Process(&api.Payload{
			TracerPayload: testutil.TracerPayloadWithChunk(testutil.TraceChunkWithSpan(spanValid)),
			Source:        info.NewReceiverStats().GetTagStats(info.Tags{}),
		})
		assert.EqualValues(0, want.TracesFiltered)
		assert.EqualValues(0, want.SpansFiltered)

		agnt.Process(&api.Payload{
			TracerPayload: testutil.TracerPayloadWithChunk(testutil.TraceChunkWithSpans([]*pb.Span{
				spanInvalid,
				spanInvalid,
			})),
			Source: want,
		})
		assert.EqualValues(1, want.TracesFiltered)
		assert.EqualValues(2, want.SpansFiltered)
	})

	t.Run("BlacklistPayload", func(t *testing.T) {
		// Regression test for DataDog/datadog-agent#6500
		cfg := config.New()
		cfg.Endpoints[0].APIKey = "test"
		cfg.Ignore["resource"] = []string{"^INSERT.*"}
		ctx, cancel := context.WithCancel(context.Background())
		agnt := NewAgent(ctx, cfg)
		defer cancel()

		now := time.Now()
		spanValid := &pb.Span{
			TraceID:  1,
			SpanID:   1,
			Resource: "SELECT name FROM people WHERE age = 42 AND extra = 55",
			Type:     "sql",
			Start:    now.Add(-time.Second).UnixNano(),
			Duration: (500 * time.Millisecond).Nanoseconds(),
		}
		spanInvalid := &pb.Span{
			TraceID:  1,
			SpanID:   1,
			Resource: "INSERT INTO db VALUES (1, 2, 3)",
			Type:     "sql",
			Start:    now.Add(-time.Second).UnixNano(),
			Duration: (500 * time.Millisecond).Nanoseconds(),
		}

		want := agnt.Receiver.Stats.GetTagStats(info.Tags{})
		assert := assert.New(t)

		agnt.Process(&api.Payload{
			TracerPayload: testutil.TracerPayloadWithChunks([]*pb.TraceChunk{
				testutil.TraceChunkWithSpans([]*pb.Span{
					spanInvalid,
					spanInvalid,
				}),
				testutil.TraceChunkWithSpan(spanValid),
			}),
			Source: want,
		})
		assert.EqualValues(1, want.TracesFiltered)
		assert.EqualValues(2, want.SpansFiltered)
		var span *pb.Span
		select {
		case ss := <-agnt.TraceWriter.In:
			span = ss.TracerPayload.Chunks[0].Spans[0]
		case <-time.After(2 * time.Second):
			t.Fatal("timeout: Expected one valid trace, but none were received.")
		}
		assert.Equal("unnamed_operation", span.Name)
	})

	t.Run("Stats/Priority", func(t *testing.T) {
		cfg := config.New()
		cfg.Endpoints[0].APIKey = "test"
		ctx, cancel := context.WithCancel(context.Background())
		agnt := NewAgent(ctx, cfg)
		defer cancel()

		want := agnt.Receiver.Stats.GetTagStats(info.Tags{})
		now := time.Now()
		for _, key := range []sampler.SamplingPriority{
			sampler.PriorityNone,
			sampler.PriorityUserDrop,
			sampler.PriorityUserDrop,
			sampler.PriorityAutoDrop,
			sampler.PriorityAutoDrop,
			sampler.PriorityAutoDrop,
			sampler.PriorityAutoKeep,
			sampler.PriorityAutoKeep,
			sampler.PriorityAutoKeep,
			sampler.PriorityAutoKeep,
			sampler.PriorityUserKeep,
			sampler.PriorityUserKeep,
			sampler.PriorityUserKeep,
			sampler.PriorityUserKeep,
			sampler.PriorityUserKeep,
		} {
			span := &pb.Span{
				TraceID:  1,
				SpanID:   1,
				Resource: "SELECT name FROM people WHERE age = 42 AND extra = 55",
				Type:     "sql",
				Start:    now.Add(-time.Second).UnixNano(),
				Duration: (500 * time.Millisecond).Nanoseconds(),
				Metrics:  map[string]float64{},
			}
			chunk := testutil.TraceChunkWithSpan(span)
			chunk.Priority = int32(key)
			agnt.Process(&api.Payload{
				TracerPayload: testutil.TracerPayloadWithChunk(chunk),
				Source:        want,
			})
		}

		samplingPriorityTagValues := want.TracesPerSamplingPriority.TagValues()
		assert.EqualValues(t, 1, want.TracesPriorityNone)
		assert.EqualValues(t, 2, samplingPriorityTagValues["-1"])
		assert.EqualValues(t, 3, samplingPriorityTagValues["0"])
		assert.EqualValues(t, 4, samplingPriorityTagValues["1"])
		assert.EqualValues(t, 5, samplingPriorityTagValues["2"])
	})

	t.Run("normalizing", func(t *testing.T) {
		cfg := config.New()
		cfg.Endpoints[0].APIKey = "test"
		ctx, cancel := context.WithCancel(context.Background())
		agnt := NewAgent(ctx, cfg)
		defer cancel()

		tp := testutil.TracerPayloadWithChunk(testutil.TraceChunkWithSpanAndPriority(&pb.Span{
			Service:  "something &&<@# that should be a metric!",
			TraceID:  1,
			SpanID:   1,
			Resource: "SELECT name FROM people WHERE age = 42 AND extra = 55",
			Type:     "sql",
			Start:    time.Now().Add(-time.Second).UnixNano(),
			Duration: (500 * time.Millisecond).Nanoseconds(),
		}, 2))
		go agnt.Process(&api.Payload{
			TracerPayload: tp,
			Source:        agnt.Receiver.Stats.GetTagStats(info.Tags{}),
		})
		timeout := time.After(2 * time.Second)
		var span *pb.Span
		select {
		case ss := <-agnt.TraceWriter.In:
			span = ss.TracerPayload.Chunks[0].Spans[0]
		case <-timeout:
			t.Fatal("timed out")
		}
		assert.Equal(t, "unnamed_operation", span.Name)
		assert.Equal(t, "something_that_should_be_a_metric", span.Service)
	})

	t.Run("_dd.hostname", func(t *testing.T) {
		cfg := config.New()
		cfg.Endpoints[0].APIKey = "test"
		ctx, cancel := context.WithCancel(context.Background())
		agnt := NewAgent(ctx, cfg)
		defer cancel()

		tp := testutil.TracerPayloadWithChunk(testutil.RandomTraceChunk(1, 1))
		tp.Chunks[0].Spans[0].Meta["_dd.hostname"] = "tracer-hostname"
		go agnt.Process(&api.Payload{
			TracerPayload: tp,
			Source:        agnt.Receiver.Stats.GetTagStats(info.Tags{}),
		})
		timeout := time.After(2 * time.Second)
		select {
		case ss := <-agnt.TraceWriter.In:
			tp = ss.TracerPayload
		case <-timeout:
			t.Fatal("timed out")
		}
		assert.Equal(t, "tracer-hostname", tp.Hostname)
	})

	t.Run("FilterSpans", func(t *testing.T) {
		cfg := config.New()
		cfg.Endpoints[0].APIKey = "test"
		ctx, cancel := context.WithCancel(context.Background())
		agnt := NewAgent(ctx, cfg)
		defer cancel()

		testFilterFunction := func(span *pb.Span) bool {
			return span.Meta["irrelevant"] == "true"
		}
		agnt.DiscardSpan = testFilterFunction

		span1 := &pb.Span{
			TraceID: 1,
			SpanID:  1,
			Service: "a",
			Meta: map[string]string{
				"irrelevant": "true",
			},
		}
		span2 := &pb.Span{TraceID: 1, SpanID: 2, Service: "a"}
		span3 := &pb.Span{TraceID: 1, SpanID: 3, Service: "a"}

		tp := testutil.TracerPayloadWithChunk(spansToChunk(span1, span2, span3))

		go agnt.Process(&api.Payload{
			TracerPayload: tp,
			Source:        agnt.Receiver.Stats.GetTagStats(info.Tags{}),
		})

		timeout := time.After(2 * time.Second)
		select {
		case ss := <-agnt.TraceWriter.In:
			assert.Equal(t, 2, int(ss.SpanCount))
			assert.NotContains(t, ss.TracerPayload.Chunks[0].Spans[0].Meta, "irrelevant")
			assert.NotContains(t, ss.TracerPayload.Chunks[0].Spans[1].Meta, "irrelevant")
		case <-timeout:
			t.Fatal("timed out")
		}
	})

	t.Run("chunking", func(t *testing.T) {
		cfg := config.New()
		cfg.Endpoints[0].APIKey = "test"
		ctx, cancel := context.WithCancel(context.Background())
		agnt := NewAgent(ctx, cfg)
		defer cancel()

		chunk1 := testutil.TraceChunkWithSpan(testutil.RandomSpan())
		chunk1.Priority = 2
		chunk2 := testutil.TraceChunkWithSpan(testutil.RandomSpan())
		chunk2.Priority = 2
		chunk3 := testutil.TraceChunkWithSpan(testutil.RandomSpan())
		chunk3.Priority = 2
		// we are sending 3 traces
		tp := testutil.TracerPayloadWithChunks([]*pb.TraceChunk{
			chunk1,
			chunk2,
			chunk3,
		})
		// setting writer.MaxPayloadSize to the size of 1 trace (+1 byte)
		defer func(oldSize int) { writer.MaxPayloadSize = oldSize }(writer.MaxPayloadSize)
		//minChunkSize := int(math.Min(math.Min(float64(tp.Chunks[0].Msgsize()), float64(tp.Chunks[1].Msgsize())), float64(tp.Chunks[2].Msgsize())))
		writer.MaxPayloadSize = 1
		// and expecting it to result in 3 payloads
		expectedPayloads := 3
		go agnt.Process(&api.Payload{
			TracerPayload: tp,
			Source:        agnt.Receiver.Stats.GetTagStats(info.Tags{}),
		})

		var gotCount int
		timeout := time.After(3 * time.Second)
		// expect multiple payloads
		for i := 0; i < expectedPayloads; i++ {
			select {
			case ss := <-agnt.TraceWriter.In:
				gotCount += int(ss.SpanCount)
			case <-timeout:
				t.Fatal("timed out")
			}
		}
		// without missing a trace
		assert.Equal(t, gotCount, 3)
	})
}

func spansToChunk(spans ...*pb.Span) *pb.TraceChunk {
	return &pb.TraceChunk{Spans: spans}
}

func dropped(c *pb.TraceChunk) *pb.TraceChunk {
	c.DroppedTrace = true
	return c
}

func TestConcentratorInput(t *testing.T) {
	rootSpan := &pb.Span{SpanID: 3, TraceID: 5, Service: "a"}
	rootSpanWithTracerTags := &pb.Span{SpanID: 3, TraceID: 5, Service: "a", Meta: map[string]string{"_dd.hostname": "host", "env": "env", "version": "version"}}
	rootSpanEvent := &pb.Span{SpanID: 3, TraceID: 5, Service: "a", Metrics: map[string]float64{"_dd1.sr.eausr": 1.00}}
	span := &pb.Span{SpanID: 3, TraceID: 5, ParentID: 27, Service: "a"}
	tts := []struct {
		name            string
		in              *api.Payload
		expected        stats.Input
		expectedSampled *pb.TracerPayload
		withFargate     bool
		features        string
	}{
		{
			name: "tracer payload tags in payload",
			in: &api.Payload{
				TracerPayload: &pb.TracerPayload{
					Hostname:   "banana",
					AppVersion: "camembert",
					Env:        "apple",
					Chunks:     []*pb.TraceChunk{spansToChunk(rootSpan)},
				},
			},
			expected: stats.Input{
				Traces: []traceutil.ProcessedTrace{
					{
						Root:           rootSpan,
						TracerHostname: "banana",
						AppVersion:     "camembert",
						TracerEnv:      "apple",
						TraceChunk:     spansToChunk(rootSpan),
					},
				},
			},
		},
		{
			name: "tracer payload tags in span",
			in: &api.Payload{
				TracerPayload: &pb.TracerPayload{
					Chunks: []*pb.TraceChunk{spansToChunk(rootSpanWithTracerTags)},
				},
			},
			expected: stats.Input{
				Traces: []traceutil.ProcessedTrace{
					{
						Root:           rootSpanWithTracerTags,
						TracerHostname: "host",
						AppVersion:     "version",
						TracerEnv:      "env",
						TraceChunk:     spansToChunk(rootSpanWithTracerTags),
					},
				},
			},
		},
		{
			name: "no tracer tags",
			in: &api.Payload{
				TracerPayload: &pb.TracerPayload{
					Chunks: []*pb.TraceChunk{spansToChunk(rootSpan)},
				},
			},
			expected: stats.Input{
				Traces: []traceutil.ProcessedTrace{
					{
						Root:       rootSpan,
						TraceChunk: spansToChunk(rootSpan),
					},
				},
			},
		},
		{
			name: "containerID with fargate orchestrator",
			in: &api.Payload{
				TracerPayload: &pb.TracerPayload{
					Chunks:      []*pb.TraceChunk{spansToChunk(rootSpan)},
					ContainerID: "aaah",
				},
			},
			withFargate: true,
			expected: stats.Input{
				Traces: []traceutil.ProcessedTrace{
					{
						Root:       rootSpan,
						TraceChunk: spansToChunk(rootSpan),
					},
				},
				ContainerID: "aaah",
			},
		},
		{
			name: "containerID no orchestrator",
			in: &api.Payload{
				TracerPayload: &pb.TracerPayload{
					Chunks:      []*pb.TraceChunk{spansToChunk(rootSpan)},
					ContainerID: "no-orch",
				},
			},
			expected: stats.Input{
				Traces: []traceutil.ProcessedTrace{
					{
						Root:       rootSpan,
						TraceChunk: spansToChunk(rootSpan),
					},
				},
			},
		},
		{
			name: "containerID feature disabled",
			in: &api.Payload{
				TracerPayload: &pb.TracerPayload{
					Chunks:      []*pb.TraceChunk{spansToChunk(rootSpan)},
					ContainerID: "feature_disabled",
				},
			},
			withFargate: true,
			features:    "disable_cid_stats",
			expected: stats.Input{
				Traces: []traceutil.ProcessedTrace{
					{
						Root:       rootSpan,
						TraceChunk: spansToChunk(rootSpan),
					},
				},
			},
		},
		{
			name: "client computed stats",
			in: &api.Payload{
				TracerPayload: &pb.TracerPayload{
					Chunks:      []*pb.TraceChunk{spansToChunk(rootSpan)},
					ContainerID: "feature_disabled",
				},
				ClientComputedStats: true,
			},
			expected: stats.Input{},
		},
		{
			name: "many chunks",
			in: &api.Payload{
				TracerPayload: &pb.TracerPayload{
					Chunks: []*pb.TraceChunk{
						spansToChunk(rootSpanWithTracerTags, span),
						spansToChunk(rootSpan),
						spansToChunk(rootSpanEvent, span),
					},
				},
			},
			expected: stats.Input{
				Traces: []traceutil.ProcessedTrace{
					{
						Root:           rootSpanWithTracerTags,
						TraceChunk:     spansToChunk(rootSpanWithTracerTags, span),
						TracerHostname: "host",
						AppVersion:     "version",
						TracerEnv:      "env",
					},
					{
						Root:           rootSpan,
						TraceChunk:     spansToChunk(rootSpan),
						TracerHostname: "host",
						AppVersion:     "version",
						TracerEnv:      "env",
					},
					{
						Root:           rootSpanEvent,
						TraceChunk:     spansToChunk(rootSpanEvent, span),
						TracerHostname: "host",
						AppVersion:     "version",
						TracerEnv:      "env",
					},
				},
			},
			expectedSampled: &pb.TracerPayload{
				Chunks:     []*pb.TraceChunk{spansToChunk(rootSpanWithTracerTags, span), dropped(spansToChunk(rootSpanEvent))},
				Env:        "env",
				Hostname:   "host",
				AppVersion: "version",
			},
		},
	}

	for _, tc := range tts {
		t.Run(tc.name, func(t *testing.T) {
			defer testutil.WithFeatures(tc.features)()
			cfg := config.New()
			cfg.Endpoints[0].APIKey = "test"
			if tc.withFargate {
				cfg.FargateOrchestrator = config.OrchestratorECS
			}
			agent := NewAgent(context.TODO(), cfg)
			tc.in.Source = agent.Receiver.Stats.GetTagStats(info.Tags{})
			agent.Process(tc.in)

			if len(tc.expected.Traces) == 0 {
				assert.Len(t, agent.Concentrator.In, 0)
				return
			}
			require.Len(t, agent.Concentrator.In, 1)
			assert.Equal(t, tc.expected, <-agent.Concentrator.In)

			if tc.expectedSampled != nil && len(tc.expectedSampled.Chunks) > 0 {
				require.Len(t, agent.TraceWriter.In, 1)
				ss := <-agent.TraceWriter.In
				assert.Equal(t, tc.expectedSampled, ss.TracerPayload)
			}
		})
	}
}

func TestClientComputedTopLevel(t *testing.T) {
	cfg := config.New()
	cfg.Endpoints[0].APIKey = "test"
	ctx, cancel := context.WithCancel(context.Background())
	agnt := NewAgent(ctx, cfg)
	defer cancel()

	t.Run("onNotTop", func(t *testing.T) {
		chunk := testutil.TraceChunkWithSpan(testutil.RandomSpan())
		chunk.Priority = 2
		tp := testutil.TracerPayloadWithChunk(chunk)
		go agnt.Process(&api.Payload{
			TracerPayload:          tp,
			Source:                 agnt.Receiver.Stats.GetTagStats(info.Tags{}),
			ClientComputedTopLevel: true,
		})
		timeout := time.After(time.Second)
		select {
		case ss := <-agnt.TraceWriter.In:
			_, ok := ss.TracerPayload.Chunks[0].Spans[0].Metrics["_top_level"]
			assert.False(t, ok)
			return
		case <-timeout:
			t.Fatal("timed out waiting for input")
		}
	})

	t.Run("off", func(t *testing.T) {
		chunk := testutil.TraceChunkWithSpan(testutil.RandomSpan())
		chunk.Priority = 2
		tp := testutil.TracerPayloadWithChunk(chunk)
		go agnt.Process(&api.Payload{
			TracerPayload:          tp,
			Source:                 agnt.Receiver.Stats.GetTagStats(info.Tags{}),
			ClientComputedTopLevel: false,
		})
		timeout := time.After(time.Second)
		select {
		case ss := <-agnt.TraceWriter.In:
			_, ok := ss.TracerPayload.Chunks[0].Spans[0].Metrics["_top_level"]
			assert.True(t, ok)
			return
		case <-timeout:
			t.Fatal("timed out waiting for input")
		}
	})

	t.Run("onTop", func(t *testing.T) {
		span := testutil.RandomSpan()
		span.Metrics = map[string]float64{
			"_dd.top_level": 1,
		}
		chunk := testutil.TraceChunkWithSpan(span)
		chunk.Priority = 2
		tp := testutil.TracerPayloadWithChunk(chunk)
		go agnt.Process(&api.Payload{
			TracerPayload:          tp,
			Source:                 agnt.Receiver.Stats.GetTagStats(info.Tags{}),
			ClientComputedTopLevel: true,
		})
		timeout := time.After(time.Second)
		select {
		case ss := <-agnt.TraceWriter.In:
			_, ok := ss.TracerPayload.Chunks[0].Spans[0].Metrics["_top_level"]
			assert.True(t, ok)
			_, ok = ss.TracerPayload.Chunks[0].Spans[0].Metrics["_dd.top_level"]
			assert.True(t, ok)
			return
		case <-timeout:
			t.Fatal("timed out waiting for input")
		}
	})
}

func TestFilteredByTags(t *testing.T) {
	for _, tt := range []struct {
		require []*config.Tag
		reject  []*config.Tag
		span    pb.Span
		drop    bool
	}{
		{
			require: []*config.Tag{{K: "key", V: "val"}},
			span:    pb.Span{Meta: map[string]string{"key": "val"}},
			drop:    false,
		},
		{
			reject: []*config.Tag{{K: "key", V: "val"}},
			span:   pb.Span{Meta: map[string]string{"key": "val4"}},
			drop:   false,
		},
		{
			reject: []*config.Tag{{K: "something", V: "else"}},
			span:   pb.Span{Meta: map[string]string{"key": "val"}},
			drop:   false,
		},
		{
			require: []*config.Tag{{K: "something", V: "else"}},
			reject:  []*config.Tag{{K: "bad-key", V: "bad-value"}},
			span:    pb.Span{Meta: map[string]string{"something": "else", "bad-key": "other-value"}},
			drop:    false,
		},
		{
			require: []*config.Tag{{K: "key", V: "value"}, {K: "key-only"}},
			reject:  []*config.Tag{{K: "bad-key", V: "bad-value"}},
			span:    pb.Span{Meta: map[string]string{"key": "value", "key-only": "but-also-value", "bad-key": "not-bad-value"}},
			drop:    false,
		},
		{
			require: []*config.Tag{{K: "key", V: "val"}},
			span:    pb.Span{Meta: map[string]string{"key": "val2"}},
			drop:    true,
		},
		{
			require: []*config.Tag{{K: "something", V: "else"}},
			span:    pb.Span{Meta: map[string]string{"key": "val"}},
			drop:    true,
		},
		{
			require: []*config.Tag{{K: "valid"}, {K: "test"}},
			reject:  []*config.Tag{{K: "test"}},
			span:    pb.Span{Meta: map[string]string{"test": "random", "valid": "random"}},
			drop:    true,
		},
		{
			require: []*config.Tag{{K: "valid-key", V: "valid-value"}, {K: "test"}},
			reject:  []*config.Tag{{K: "test"}},
			span:    pb.Span{Meta: map[string]string{"test": "random", "valid-key": "wrong-value"}},
			drop:    true,
		},
		{
			reject: []*config.Tag{{K: "key", V: "val"}},
			span:   pb.Span{Meta: map[string]string{"key": "val"}},
			drop:   true,
		},
		{
			require: []*config.Tag{{K: "something", V: "else"}, {K: "key-only"}},
			reject:  []*config.Tag{{K: "bad-key", V: "bad-value"}, {K: "bad-key-only"}},
			span:    pb.Span{Meta: map[string]string{"something": "else", "key-only": "but-also-value", "bad-key-only": "random"}},
			drop:    true,
		},
	} {
		t.Run("", func(t *testing.T) {
			if filteredByTags(&tt.span, tt.require, tt.reject) != tt.drop {
				t.Fatal()
			}
		})
	}
}

func TestClientComputedStats(t *testing.T) {
	cfg := config.New()
	cfg.Endpoints[0].APIKey = "test"
	ctx, cancel := context.WithCancel(context.Background())
	agnt := NewAgent(ctx, cfg)
	defer cancel()
	tp := testutil.TracerPayloadWithChunk(testutil.TraceChunkWithSpanAndPriority(&pb.Span{
		Service:  "something &&<@# that should be a metric!",
		TraceID:  1,
		SpanID:   1,
		Resource: "SELECT name FROM people WHERE age = 42 AND extra = 55",
		Type:     "sql",
		Start:    time.Now().Add(-time.Second).UnixNano(),
		Duration: (500 * time.Millisecond).Nanoseconds(),
	}, 2))

	t.Run("on", func(t *testing.T) {
		agnt.Process(&api.Payload{
			TracerPayload:       tp,
			Source:              agnt.Receiver.Stats.GetTagStats(info.Tags{}),
			ClientComputedStats: true,
		})
		assert.Len(t, agnt.Concentrator.In, 0)
	})

	t.Run("off", func(t *testing.T) {
		agnt.Process(&api.Payload{
			TracerPayload:       tp,
			Source:              agnt.Receiver.Stats.GetTagStats(info.Tags{}),
			ClientComputedStats: false,
		})
		assert.Len(t, agnt.Concentrator.In, 1)
	})
}

func TestSampling(t *testing.T) {
	for name, tt := range map[string]struct {
		// hasErrors will be true if the input trace should have errors
		// hasPriority will be true if the input trace should have sampling priority set
		hasErrors, hasPriority bool

		// noPrioritySampled, errorsSampled, prioritySampled are the sample decisions of the mock samplers
		noPrioritySampled, errorsSampled, prioritySampled bool

		// disableRareSampler disables the rare sampler by configuration
		disableRareSampler bool

		// wantSampled is the expected result
		wantSampled bool
	}{
		"nopriority-unsampled": {
			noPrioritySampled: false,
			wantSampled:       false,
		},
		"nopriority-sampled": {
			noPrioritySampled: true,
			wantSampled:       true,
		},
		"prio-unsampled": {
			hasPriority:        true,
			prioritySampled:    false,
			disableRareSampler: true,
			wantSampled:        false,
		},
		"prio-sampled": {
			hasPriority:     true,
			prioritySampled: true,
			wantSampled:     true,
		},
		"error-unsampled": {
			hasErrors:     true,
			errorsSampled: false,
			wantSampled:   false,
		},
		"error-sampled": {
			hasErrors:     true,
			errorsSampled: true,
			wantSampled:   true,
		},
		"error-sampled-prio-unsampled": {
			hasErrors:       true,
			hasPriority:     true,
			errorsSampled:   true,
			prioritySampled: false,
			wantSampled:     true,
		},
		"error-unsampled-prio-sampled": {
			hasErrors:       true,
			hasPriority:     true,
			errorsSampled:   false,
			prioritySampled: true,
			wantSampled:     true,
		},
		"error-prio-sampled": {
			hasErrors:       true,
			hasPriority:     true,
			errorsSampled:   true,
			prioritySampled: true,
			wantSampled:     true,
		},
		"error-prio-unsampled": {
			hasErrors:       true,
			hasPriority:     true,
			errorsSampled:   false,
			prioritySampled: false,
			wantSampled:     false,
		},
		"rare-sampler-catch": {
			hasPriority: true,
			wantSampled: true,
		},
		"rare-sampler-disabled": {
			hasPriority:        true,
			disableRareSampler: true,
			wantSampled:        false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := &config.AgentConfig{DisableRareSampler: tt.disableRareSampler}
			sampledCfg := &config.AgentConfig{ExtraSampleRate: 1, TargetTPS: 5, ErrorTPS: 10, DisableRareSampler: tt.disableRareSampler}

			a := &Agent{
				NoPrioritySampler: sampler.NewNoPrioritySampler(cfg),
				ErrorsSampler:     sampler.NewErrorsSampler(cfg),
				PrioritySampler:   sampler.NewPrioritySampler(cfg, &sampler.DynamicConfig{}),
				RareSampler:       sampler.NewRareSampler(),
				conf:              cfg,
			}
			if tt.errorsSampled {
				a.ErrorsSampler = sampler.NewErrorsSampler(sampledCfg)
			}
			if tt.noPrioritySampled {
				a.NoPrioritySampler = sampler.NewNoPrioritySampler(sampledCfg)
			}

			root := &pb.Span{
				Service:  "serv1",
				Start:    time.Now().UnixNano(),
				Duration: (100 * time.Millisecond).Nanoseconds(),
				Metrics:  map[string]float64{"_top_level": 1},
			}

			if tt.hasErrors {
				root.Error = 1
			}
			pt := traceutil.ProcessedTrace{TraceChunk: testutil.TraceChunkWithSpan(root), Root: root}
			if tt.hasPriority {
				if tt.prioritySampled {
					pt.TraceChunk.Priority = 1
				} else {
					pt.TraceChunk.Priority = 0
				}
			}

			sampled := a.runSamplers(time.Now(), pt, tt.hasPriority)
			assert.EqualValues(t, tt.wantSampled, sampled)
		})
	}
}

func TestPartialSamplingFree(t *testing.T) {
	cfg := &config.AgentConfig{DisableRareSampler: true, BucketInterval: 10 * time.Second}
	statsChan := make(chan pb.StatsPayload, 100)
	writerChan := make(chan *writer.SampledChunks, 100)
	dynConf := sampler.NewDynamicConfig()
	in := make(chan *api.Payload, 1000)
	agnt := &Agent{
		Concentrator:      stats.NewConcentrator(cfg, statsChan, time.Now()),
		Blacklister:       filters.NewBlacklister(cfg.Ignore["resource"]),
		Replacer:          filters.NewReplacer(cfg.ReplaceTags),
		NoPrioritySampler: sampler.NewNoPrioritySampler(cfg),
		ErrorsSampler:     sampler.NewErrorsSampler(cfg),
		PrioritySampler:   sampler.NewPrioritySampler(cfg, &sampler.DynamicConfig{}),
		EventProcessor:    newEventProcessor(cfg),
		RareSampler:       sampler.NewRareSampler(),
		TraceWriter:       &writer.TraceWriter{In: writerChan},
		conf:              cfg,
	}
	agnt.Receiver = api.NewHTTPReceiver(cfg, dynConf, in, agnt)
	now := time.Now()
	smallKeptSpan := &pb.Span{
		TraceID:  1,
		SpanID:   1,
		Service:  "s",
		Name:     "n",
		Resource: "aaaa",
		Type:     "web",
		Start:    now.Add(-time.Second).UnixNano(),
		Duration: (500 * time.Millisecond).Nanoseconds(),
		Metrics:  map[string]float64{"_sampling_priority_v1": 1.0},
	}

	tracerPayload := testutil.TracerPayloadWithChunk(testutil.TraceChunkWithSpan(smallKeptSpan))

	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	assert.Less(t, m.HeapInuse, uint64(50*1e6))

	droppedSpan := &pb.Span{
		TraceID:  1,
		SpanID:   1,
		Service:  "s",
		Name:     "n",
		Resource: "bbb",
		Type:     "web",
		Start:    now.Add(-time.Second).UnixNano(),
		Duration: (500 * time.Millisecond).Nanoseconds(),
		Metrics:  map[string]float64{"_sampling_priority_v1": 0.0},
		Meta:     map[string]string{},
	}
	for i := 0; i < 5*1e3; i++ {
		droppedSpan.Meta[strconv.Itoa(i)] = strings.Repeat("0123456789", 1e3)
	}
	bigChunk := testutil.TraceChunkWithSpan(droppedSpan)
	bigChunk.Origin = strings.Repeat("0123456789", 50*1e6)
	tracerPayload.Chunks = append(tracerPayload.Chunks, bigChunk)
	runtime.GC()
	runtime.ReadMemStats(&m)
	assert.Greater(t, m.HeapInuse, uint64(50*1e6))
	agnt.Process(&api.Payload{
		TracerPayload: tracerPayload,
		Source:        info.NewReceiverStats().GetTagStats(info.Tags{}),
	})
	runtime.GC()
	runtime.ReadMemStats(&m)
	assert.Greater(t, m.HeapInuse, uint64(50*1e6))

	<-agnt.Concentrator.In
	// big chunk should be cleaned as unsampled and passed through stats
	runtime.GC()
	runtime.ReadMemStats(&m)
	assert.Less(t, m.HeapInuse, uint64(50*1e6))

	p := <-agnt.TraceWriter.In
	assert.Len(t, p.TracerPayload.Chunks, 1)
}

func TestEventProcessorFromConf(t *testing.T) {
	if _, ok := os.LookupEnv("INTEGRATION"); !ok {
		t.Skip("set INTEGRATION environment variable to run")
	}
	if testing.Short() {
		return
	}
	testMaxEPS := 100.
	rateByServiceAndName := map[string]map[string]float64{
		"serviceA": {
			"opA": 0,
			"opC": 1,
		},
		"serviceB": {
			"opB": 0.5,
		},
	}
	rateByService := map[string]float64{
		"serviceA": 1,
		"serviceC": 0.5,
		"serviceD": 1,
	}

	for _, testCase := range []eventProcessorTestCase{
		// Name: <extractor>/<maxeps situation>/priority
		{name: "none/below/none", intakeSPS: 100, serviceName: "serviceE", opName: "opA", extractionRate: -1, priority: sampler.PriorityNone, expectedEPS: 0, deltaPct: 0, duration: 10 * time.Second},
		{name: "metric/below/none", intakeSPS: 100, serviceName: "serviceD", opName: "opA", extractionRate: 0.5, priority: sampler.PriorityNone, expectedEPS: 50, deltaPct: 0.1, duration: 10 * time.Second},
		{name: "metric/above/none", intakeSPS: 200, serviceName: "serviceD", opName: "opA", extractionRate: 1, priority: sampler.PriorityNone, expectedEPS: 100, deltaPct: 0.5, duration: 60 * time.Second},
		{name: "fixed/below/none", intakeSPS: 100, serviceName: "serviceB", opName: "opB", extractionRate: -1, priority: sampler.PriorityNone, expectedEPS: 50, deltaPct: 0.1, duration: 10 * time.Second},
		{name: "fixed/above/none", intakeSPS: 200, serviceName: "serviceA", opName: "opC", extractionRate: -1, priority: sampler.PriorityNone, expectedEPS: 100, deltaPct: 0.5, duration: 60 * time.Second},
		{name: "fixed/above/autokeep", intakeSPS: 200, serviceName: "serviceA", opName: "opC", extractionRate: -1, priority: sampler.PriorityAutoKeep, expectedEPS: 100, deltaPct: 0.5, duration: 60 * time.Second},
		{name: "metric/above/autokeep", intakeSPS: 200, serviceName: "serviceD", opName: "opA", extractionRate: 1, priority: sampler.PriorityAutoKeep, expectedEPS: 100, deltaPct: 0.5, duration: 60 * time.Second},
		// UserKeep traces allows overflow of EPS
		{name: "metric/above/userkeep", intakeSPS: 200, serviceName: "serviceD", opName: "opA", extractionRate: 1, priority: sampler.PriorityUserKeep, expectedEPS: 200, deltaPct: 0.1, duration: 10 * time.Second},
		{name: "agent/above/userkeep", intakeSPS: 200, serviceName: "serviceA", opName: "opC", extractionRate: -1, priority: sampler.PriorityUserKeep, expectedEPS: 200, deltaPct: 0.1, duration: 10 * time.Second},

		// Overrides (Name: <extractor1>/override/<extractor2>)
		{name: "metric/override/fixed", intakeSPS: 100, serviceName: "serviceA", opName: "opA", extractionRate: 1, priority: sampler.PriorityNone, expectedEPS: 100, deltaPct: 0.1, duration: 10 * time.Second},
		// Legacy should never be considered if fixed rate is being used.
		{name: "fixed/override/legacy", intakeSPS: 100, serviceName: "serviceA", opName: "opD", extractionRate: -1, priority: sampler.PriorityNone, expectedEPS: 0, deltaPct: 0, duration: 10 * time.Second},
	} {
		testEventProcessorFromConf(t, &config.AgentConfig{
			MaxEPS:                      testMaxEPS,
			AnalyzedSpansByService:      rateByServiceAndName,
			AnalyzedRateByServiceLegacy: rateByService,
		}, testCase)
	}
}

func TestEventProcessorFromConfLegacy(t *testing.T) {
	if _, ok := os.LookupEnv("INTEGRATION"); !ok {
		t.Skip("set INTEGRATION environment variable to run")
	}

	testMaxEPS := 100.

	rateByService := map[string]float64{
		"serviceA": 1,
		"serviceC": 0.5,
		"serviceD": 1,
	}

	for _, testCase := range []eventProcessorTestCase{
		// Name: <extractor>/<maxeps situation>/priority
		{name: "none/below/none", intakeSPS: 100, serviceName: "serviceE", opName: "opA", extractionRate: -1, priority: sampler.PriorityNone, expectedEPS: 0, deltaPct: 0, duration: 10 * time.Second},
		{name: "legacy/below/none", intakeSPS: 100, serviceName: "serviceC", opName: "opB", extractionRate: -1, priority: sampler.PriorityNone, expectedEPS: 50, deltaPct: 0.1, duration: 10 * time.Second},
		{name: "legacy/above/none", intakeSPS: 200, serviceName: "serviceD", opName: "opC", extractionRate: -1, priority: sampler.PriorityNone, expectedEPS: 100, deltaPct: 0.5, duration: 60 * time.Second},
		{name: "legacy/above/autokeep", intakeSPS: 200, serviceName: "serviceD", opName: "opC", extractionRate: -1, priority: sampler.PriorityAutoKeep, expectedEPS: 100, deltaPct: 0.5, duration: 60 * time.Second},
		// UserKeep traces allows overflow of EPS
		{name: "legacy/above/userkeep", intakeSPS: 200, serviceName: "serviceD", opName: "opC", extractionRate: -1, priority: sampler.PriorityUserKeep, expectedEPS: 200, deltaPct: 0.1, duration: 10 * time.Second},

		// Overrides (Name: <extractor1>/override/<extractor2>)
		{name: "metrics/overrides/legacy", intakeSPS: 100, serviceName: "serviceC", opName: "opC", extractionRate: 1, priority: sampler.PriorityNone, expectedEPS: 100, deltaPct: 0.1, duration: 10 * time.Second},
	} {
		testEventProcessorFromConf(t, &config.AgentConfig{
			MaxEPS:                      testMaxEPS,
			AnalyzedRateByServiceLegacy: rateByService,
		}, testCase)
	}
}

type eventProcessorTestCase struct {
	name           string
	intakeSPS      float64
	serviceName    string
	opName         string
	extractionRate float64
	priority       sampler.SamplingPriority
	expectedEPS    float64
	deltaPct       float64
	duration       time.Duration
}

func testEventProcessorFromConf(t *testing.T, conf *config.AgentConfig, testCase eventProcessorTestCase) {
	t.Run(testCase.name, func(t *testing.T) {
		processor := newEventProcessor(conf)
		processor.Start()

		actualEPS := generateTraffic(processor, testCase.serviceName, testCase.opName, testCase.extractionRate,
			testCase.duration, testCase.intakeSPS, testCase.priority)

		processor.Stop()

		assert.InDelta(t, testCase.expectedEPS, actualEPS, testCase.expectedEPS*testCase.deltaPct)
	})
}

// generateTraffic generates traces every 100ms with enough spans to meet the desired `intakeSPS` (intake spans per
// second). These spans will all have the provided service and operation names and be set as extractable/sampled
// based on the associated rate/%. This traffic generation will run for the specified `duration`.
func generateTraffic(processor *event.Processor, serviceName string, operationName string, extractionRate float64,
	duration time.Duration, intakeSPS float64, priority sampler.SamplingPriority) float64 {
	tickerInterval := 100 * time.Millisecond
	totalSampled := 0
	timer := time.NewTimer(duration)
	eventTicker := time.NewTicker(tickerInterval)
	defer eventTicker.Stop()
	numTicksInSecond := float64(time.Second) / float64(tickerInterval)
	spansPerTick := int(math.Round(intakeSPS / numTicksInSecond))

Loop:
	for {
		spans := make([]*pb.Span, spansPerTick)
		for i := range spans {
			span := testutil.RandomSpan()
			span.Service = serviceName
			span.Name = operationName
			if extractionRate >= 0 {
				span.Metrics[sampler.KeySamplingRateEventExtraction] = extractionRate
			}
			traceutil.SetTopLevel(span, true)
			spans[i] = span
		}
		root := spans[0]
		chunk := testutil.TraceChunkWithSpans(spans)
		chunk.Priority = int32(priority)

		numEvents, _ := processor.Process(root, chunk)
		totalSampled += int(numEvents)

		<-eventTicker.C
		select {
		case <-timer.C:
			// If timer ran out, break out of loop and stop generation
			break Loop
		default:
			// Otherwise, lets generate another
		}
	}
	return float64(totalSampled) / duration.Seconds()
}

func BenchmarkAgentTraceProcessing(b *testing.B) {
	c := config.New()
	c.Endpoints[0].APIKey = "test"

	runTraceProcessingBenchmark(b, c)
}

func BenchmarkAgentTraceProcessingWithFiltering(b *testing.B) {
	c := config.New()
	c.Endpoints[0].APIKey = "test"
	c.Ignore["resource"] = []string{"[0-9]{3}", "foobar", "G.T [a-z]+", "[^123]+_baz"}

	runTraceProcessingBenchmark(b, c)
}

// worst case scenario: spans are tested against multiple rules without any match.
// this means we won't compesate the overhead of filtering by dropping traces
func BenchmarkAgentTraceProcessingWithWorstCaseFiltering(b *testing.B) {
	c := config.New()
	c.Endpoints[0].APIKey = "test"
	c.Ignore["resource"] = []string{"[0-9]{3}", "foobar", "aaaaa?aaaa", "[^123]+_baz"}

	runTraceProcessingBenchmark(b, c)
}

func runTraceProcessingBenchmark(b *testing.B, c *config.AgentConfig) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	ta := NewAgent(ctx, c)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		ta.Process(&api.Payload{
			TracerPayload: testutil.TracerPayloadWithChunk(testutil.RandomTraceChunk(10, 8)),
			Source:        info.NewReceiverStats().GetTagStats(info.Tags{}),
		})
	}
}

// Mimicks behaviour of agent Process function
func formatTrace(t pb.Trace) pb.Trace {
	for _, span := range t {
		(&Agent{obfuscator: obfuscate.NewObfuscator(obfuscate.Config{})}).obfuscateSpan(span)
		Truncate(span)
	}
	return t
}

func BenchmarkThroughput(b *testing.B) {
	env, ok := os.LookupEnv("DD_TRACE_TEST_FOLDER")
	if !ok {
		b.SkipNow()
	}

	log.SetLogger(log.NoopLogger) // disable logging

	folder := filepath.Join(env, "benchmarks")
	filepath.Walk(folder, func(path string, info os.FileInfo, err error) error {
		ext := filepath.Ext(path)
		if ext != ".msgp" {
			return nil
		}
		b.Run(info.Name(), benchThroughput(path))
		return nil
	})
}

func benchThroughput(file string) func(*testing.B) {
	return func(b *testing.B) {
		data, count, err := tracesFromFile(file)
		if err != nil {
			b.Fatal(err)
		}
		cfg := config.New()
		cfg.Endpoints[0].APIKey = "irrelevant"

		ctx, cancelFunc := context.WithCancel(context.Background())
		http.DefaultServeMux = &http.ServeMux{}
		agnt := NewAgent(ctx, cfg)
		defer cancelFunc()

		// start the agent without the trace and stats writers; we will be draining
		// these channels ourselves in the benchmarks, plus we don't want the writers
		// resource usage to show up in the results.
		agnt.TraceWriter.In = make(chan *writer.SampledChunks)
		go agnt.Run()

		// wait for receiver to start:
		for {
			resp, err := http.Get("http://localhost:8126/v0.4/traces")
			if err != nil {
				time.Sleep(time.Millisecond)
				continue
			}
			if resp.StatusCode == 400 {
				break
			}
		}

		// drain every other channel to avoid blockage.
		exit := make(chan bool)
		go func() {
			defer close(exit)
			for {
				select {
				case <-agnt.Concentrator.Out:
				case <-exit:
					return
				}
			}
		}()

		b.ResetTimer()
		b.SetBytes(int64(len(data)))

		for i := 0; i < b.N; i++ {
			req, err := http.NewRequest("PUT", "http://localhost:8126/v0.4/traces", bytes.NewReader(data))
			if err != nil {
				b.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/msgpack")
			w := httptest.NewRecorder()

			// create the request by calling directly into the Handler;
			// we are not interested in benchmarking HTTP latency. This
			// also ensures we avoid potential connection failures that
			// would make the benchmarks inconsistent.
			http.DefaultServeMux.ServeHTTP(w, req)
			if w.Code != 200 {
				b.Fatalf("%d: %v", i, w.Body.String())
			}

			var got int
			timeout := time.After(1 * time.Second)
		loop:
			for {
				select {
				case <-agnt.TraceWriter.In:
					got++
					if got == count {
						// processed everything!
						break loop
					}
				case <-timeout:
					// taking too long...
					b.Fatalf("time out at %d/%d", got, count)
					break loop
				}
			}
		}

		exit <- true
		<-exit
	}
}

// tracesFromFile extracts raw msgpack data from the given file, modifying each trace
// to have sampling.priority=2 to guarantee consistency. It also returns the amount of
// traces found and any error in obtaining the information.
func tracesFromFile(file string) (raw []byte, count int, err error) {
	if file[0] != '/' {
		file = filepath.Join(os.Getenv("GOPATH"), file)
	}
	in, err := os.Open(file)
	if err != nil {
		return nil, 0, err
	}
	defer in.Close()
	// prepare the traces in this file by adding sampling.priority=2
	// everywhere to ensure consistent sampling assumptions and results.
	var traces pb.Traces
	bts, err := ioutil.ReadAll(in)
	if _, err = traces.UnmarshalMsg(bts); err != nil {
		return nil, 0, err
	}
	for _, t := range traces {
		count++
		for _, s := range t {
			if s.Metrics == nil {
				s.Metrics = map[string]float64{"_sampling_priority_v1": 2}
			} else {
				s.Metrics["_sampling_priority_v1"] = 2
			}
		}
	}
	// re-encode the modified payload
	var data []byte
	if data, err = traces.MarshalMsg(nil); err != nil {
		return nil, 0, err
	}
	return data, count, nil
}

func TestConvertStats(t *testing.T) {
	testCases := []struct {
		in            pb.ClientStatsPayload
		lang          string
		tracerVersion string
		out           pb.ClientStatsPayload
	}{
		{
			in: pb.ClientStatsPayload{
				Hostname: "tracer_hots",
				Env:      "tracer_env",
				Version:  "code_version",
				Stats: []pb.ClientStatsBucket{
					{
						Start:    1,
						Duration: 2,
						Stats: []pb.ClientGroupedStats{
							{
								Service:        "service",
								Name:           "name------",
								Resource:       "resource",
								HTTPStatusCode: 400,
								Type:           "web",
							},
							{
								Service:        "service",
								Name:           "name",
								Resource:       "blocked_resource",
								HTTPStatusCode: 400,
								Type:           "web",
							},
							{
								Service:        "redis_service",
								Name:           "name-2",
								Resource:       "SET k v",
								HTTPStatusCode: 400,
								Type:           "redis",
							},
						},
					},
				},
			},
			lang:          "java",
			tracerVersion: "v1",
			out: pb.ClientStatsPayload{
				Hostname:      "tracer_hots",
				Env:           "tracer_env",
				Version:       "code_version",
				Lang:          "java",
				TracerVersion: "v1",
				Stats: []pb.ClientStatsBucket{
					{
						Start:    1,
						Duration: 2,
						Stats: []pb.ClientGroupedStats{
							{
								Service:        "service",
								Name:           "name",
								Resource:       "resource",
								HTTPStatusCode: 200,
								Type:           "web",
							},
							{
								Service:        "redis_service",
								Name:           "name_2",
								Resource:       "SET",
								HTTPStatusCode: 200,
								Type:           "redis",
							},
						},
					},
				},
			},
		},
	}
	a := Agent{
		Blacklister: filters.NewBlacklister([]string{"blocked_resource"}),
		obfuscator:  obfuscate.NewObfuscator(obfuscate.Config{}),
		Replacer:    filters.NewReplacer([]*config.ReplaceRule{{Name: "http.status_code", Pattern: "400", Re: regexp.MustCompile("400"), Repl: "200"}}),
		conf:        &config.AgentConfig{DefaultEnv: "agent_env", Hostname: "agent_hostname"},
	}
	for _, testCase := range testCases {
		out := a.processStats(testCase.in, testCase.lang, testCase.tracerVersion)
		assert.Equal(t, testCase.out, out)
	}
}

func TestMergeDuplicates(t *testing.T) {
	in := pb.ClientStatsBucket{
		Stats: []pb.ClientGroupedStats{
			{
				Service:      "s1",
				Resource:     "r1",
				Name:         "n1",
				Hits:         2,
				TopLevelHits: 2,
				Errors:       1,
				Duration:     123,
			},
			{
				Service:      "s2",
				Resource:     "r1",
				Name:         "n1",
				Hits:         2,
				TopLevelHits: 2,
				Errors:       0,
				Duration:     123,
			},
			{
				Service:      "s1",
				Resource:     "r1",
				Name:         "n1",
				Hits:         2,
				TopLevelHits: 2,
				Errors:       1,
				Duration:     123,
			},
			{
				Service:      "s2",
				Resource:     "r1",
				Name:         "n1",
				Hits:         2,
				TopLevelHits: 2,
				Errors:       0,
				Duration:     123,
			},
		},
	}
	expected := pb.ClientStatsBucket{
		Stats: []pb.ClientGroupedStats{
			{
				Service:      "s1",
				Resource:     "r1",
				Name:         "n1",
				Hits:         4,
				TopLevelHits: 2,
				Errors:       2,
				Duration:     246,
			},
			{
				Service:      "s2",
				Resource:     "r1",
				Name:         "n1",
				Hits:         4,
				TopLevelHits: 2,
				Errors:       0,
				Duration:     246,
			},
			{
				Service:      "s1",
				Resource:     "r1",
				Name:         "n1",
				Hits:         0,
				TopLevelHits: 2,
				Errors:       0,
				Duration:     0,
			},
			{
				Service:      "s2",
				Resource:     "r1",
				Name:         "n1",
				Hits:         0,
				TopLevelHits: 2,
				Errors:       0,
				Duration:     0,
			},
		},
	}
	mergeDuplicates(in)
	assert.Equal(t, expected, in)
}

func TestSampleWithPriorityNone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg := config.New()
	cfg.Endpoints[0].APIKey = "test"
	agnt := NewAgent(ctx, cfg)
	defer cancel()

	span := testutil.RandomSpan()
	numEvents, keep, _ := agnt.sample(time.Now(), info.NewReceiverStats().GetTagStats(info.Tags{}), traceutil.ProcessedTrace{
		TraceChunk: testutil.TraceChunkWithSpan(span),
		Root:       span,
	})
	assert.True(t, keep) // Score Sampler should keep the trace.
	assert.EqualValues(t, numEvents, 0)
}
