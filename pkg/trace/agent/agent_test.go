// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	gzip "github.com/DataDog/datadog-agent/comp/trace/compression/impl-gzip"

	"github.com/DataDog/datadog-agent/pkg/obfuscate"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/event"
	"github.com/DataDog/datadog-agent/pkg/trace/filters"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/trace/stats"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
	"github.com/DataDog/datadog-agent/pkg/trace/timing"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
	"github.com/DataDog/datadog-agent/pkg/trace/writer"
	mockStatsd "github.com/DataDog/datadog-go/v5/statsd/mocks"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-go/v5/statsd"
)

func NewTestAgent(ctx context.Context, conf *config.AgentConfig, telemetryCollector telemetry.TelemetryCollector) *Agent {
	a := NewAgent(ctx, conf, telemetryCollector, &statsd.NoOpClient{}, gzip.NewComponent())
	a.Concentrator = &mockConcentrator{}
	a.TraceWriter = &mockTraceWriter{
		apiKey: conf.Endpoints[0].APIKey,
	}
	return a
}

type mockTraceWriter struct {
	mu       sync.Mutex
	payloads []*writer.SampledChunks

	apiKey string
}

func (m *mockTraceWriter) Stop() {}

func (m *mockTraceWriter) WriteChunks(pkg *writer.SampledChunks) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.payloads = append(m.payloads, pkg)
}

func (m *mockTraceWriter) FlushSync() error {
	panic("not implemented")
}

func (m *mockTraceWriter) UpdateAPIKey(_, newKey string) {
	m.apiKey = newKey
}

type mockConcentrator struct {
	stats []stats.Input
	mu    sync.Mutex
}

func (c *mockConcentrator) Start() {}
func (c *mockConcentrator) Stop()  {}
func (c *mockConcentrator) Add(t stats.Input) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stats = append(c.stats, t)
}
func (c *mockConcentrator) Reset() []stats.Input {
	c.mu.Lock()
	defer c.mu.Unlock()
	ret := c.stats
	c.stats = nil
	return ret
}

type mockTracerPayloadModifier struct {
	modifyCalled bool
	lastPayload  *pb.TracerPayload
}

func (m *mockTracerPayloadModifier) Modify(tp *pb.TracerPayload) {
	m.modifyCalled = true
	m.lastPayload = tp
}

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

func TestStopWaits(t *testing.T) {
	cfg := config.New()
	cfg.Endpoints[0].APIKey = "test"
	cfg.Obfuscation.Cache.Enabled = true
	cfg.Obfuscation.Cache.MaxSize = 1_000
	// Disable the HTTP server to avoid colliding with a real agent on CI machines
	cfg.ReceiverPort = 0
	// But keep a ReceiverSocket so that the Receiver can start and shutdown normally
	cfg.ReceiverSocket = t.TempDir() + "/trace-agent-test.sock"
	ctx, cancel := context.WithCancel(context.Background())
	agnt := NewTestAgent(ctx, cfg, telemetry.NewNoopCollector())

	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		agnt.Run()
	}()

	now := time.Now()
	span := &pb.Span{
		TraceID:  1,
		SpanID:   1,
		Resource: "SELECT name FROM people WHERE age = 42 AND extra = 55",
		Type:     "sql",
		Start:    now.Add(-time.Second).UnixNano(),
		Duration: (500 * time.Millisecond).Nanoseconds(),
	}

	// Use select to avoid blocking if channel is closed
	payload := &api.Payload{
		TracerPayload: testutil.TracerPayloadWithChunk(testutil.TraceChunkWithSpan(span)),
		Source:        info.NewReceiverStats().GetTagStats(info.Tags{}),
	}

	select {
	case agnt.In <- payload:
		// Successfully sent payload
	case <-ctx.Done():
		// Context cancelled before we could send
		t.Fatal("Context cancelled before payload could be sent")
	case <-time.After(100 * time.Millisecond):
		// Timeout - this shouldn't happen in normal operation
		t.Fatal("Timeout sending payload to agent")
	}

	cancel()
	wg.Wait() // Wait for agent to completely exit

	mtw, ok := agnt.TraceWriter.(*mockTraceWriter)
	if !ok {
		t.Fatal("Expected mockTraceWriter")
	}
	mtw.mu.Lock()
	defer mtw.mu.Unlock()

	assert := assert.New(t)
	assert.Len(mtw.payloads, 1)
	assert.Equal("SELECT name FROM people WHERE age = ? AND extra = ?", mtw.payloads[0].TracerPayload.Chunks[0].Spans[0].Meta["sql.query"])
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
		agnt := NewTestAgent(ctx, cfg, telemetry.NewNoopCollector())
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

	t.Run("TracerPayloadModifier", func(t *testing.T) {
		cfg := config.New()
		cfg.Endpoints[0].APIKey = "test"
		ctx, cancel := context.WithCancel(context.Background())
		agnt := NewTestAgent(ctx, cfg, telemetry.NewNoopCollector())
		defer cancel()

		// Create a mock TracerPayloadModifier that tracks calls
		mockModifier := &mockTracerPayloadModifier{}
		agnt.TracerPayloadModifier = mockModifier

		now := time.Now()
		span := &pb.Span{
			TraceID:  1,
			SpanID:   1,
			Resource: "test-resource",
			Start:    now.Add(-time.Second).UnixNano(),
			Duration: (500 * time.Millisecond).Nanoseconds(),
		}

		agnt.Process(&api.Payload{
			TracerPayload: testutil.TracerPayloadWithChunk(testutil.TraceChunkWithSpan(span)),
			Source:        info.NewReceiverStats().GetTagStats(info.Tags{}),
		})

		assert := assert.New(t)
		assert.True(mockModifier.modifyCalled, "TracerPayloadModifier.Modify should have been called")
		assert.NotNil(mockModifier.lastPayload, "TracerPayloadModifier should have received a payload")
	})

	t.Run("ReplacerMetrics", func(t *testing.T) {
		cfg := config.New()
		cfg.Endpoints[0].APIKey = "test"
		cfg.ReplaceTags = []*config.ReplaceRule{
			{
				Name: "request.zipcode",
				Re:   regexp.MustCompile(".*"),
				Repl: "...",
			},
			{
				Name: "*",
				Re:   regexp.MustCompile("1337"),
				Repl: "5555",
			},
		}
		ctx, cancel := context.WithCancel(context.Background())
		agnt := NewTestAgent(ctx, cfg, telemetry.NewNoopCollector())
		defer cancel()

		now := time.Now()
		span := &pb.Span{
			TraceID:  1,
			SpanID:   1,
			Resource: "resource",
			Type:     "web",
			Start:    now.Add(-time.Second).UnixNano(),
			Duration: (500 * time.Millisecond).Nanoseconds(),
			Metrics:  map[string]float64{"request.zipcode": 12345, "request.secret": 1337, "safe.data": 42},
			Meta:     map[string]string{"keep.me": "very-normal-not-sensitive-data"},
		}
		agnt.Process(&api.Payload{
			TracerPayload: testutil.TracerPayloadWithChunk(testutil.TraceChunkWithSpan(span)),
			Source:        info.NewReceiverStats().GetTagStats(info.Tags{}),
		})

		assert.Equal(t, 5555.0, span.Metrics["request.secret"])
		assert.Equal(t, "...", span.Meta["request.zipcode"])
		assert.Equal(t, "very-normal-not-sensitive-data", span.Meta["keep.me"])
		assert.Equal(t, 42.0, span.Metrics["safe.data"])
	})

	t.Run("Blacklister", func(t *testing.T) {
		cfg := config.New()
		cfg.Endpoints[0].APIKey = "test"
		cfg.Ignore["resource"] = []string{"^INSERT.*"}
		ctx, cancel := context.WithCancel(context.Background())
		agnt := NewTestAgent(ctx, cfg, telemetry.NewNoopCollector())
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
			Source:        want,
		})
		assert.EqualValues(0, want.TracesFiltered.Load())
		assert.EqualValues(0, want.SpansFiltered.Load())

		agnt.Process(&api.Payload{
			TracerPayload: testutil.TracerPayloadWithChunk(testutil.TraceChunkWithSpans([]*pb.Span{
				spanInvalid,
				spanInvalid,
			})),
			Source: want,
		})
		assert.EqualValues(1, want.TracesFiltered.Load())
		assert.EqualValues(2, want.SpansFiltered.Load())
	})

	t.Run("Block-all", func(t *testing.T) {
		cfg := config.New()
		cfg.Endpoints[0].APIKey = "test"
		cfg.Ignore["resource"] = []string{".*"}
		ctx, cancel := context.WithCancel(context.Background())
		agnt := NewTestAgent(ctx, cfg, telemetry.NewNoopCollector())
		defer cancel()

		now := time.Now()
		span1 := &pb.Span{
			TraceID:  1,
			SpanID:   1,
			Resource: "SELECT name FROM people WHERE age = 42 AND extra = 55",
			Type:     "sql",
			Start:    now.Add(-time.Second).UnixNano(),
			Duration: (500 * time.Millisecond).Nanoseconds(),
		}
		span2 := &pb.Span{
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
			TracerPayload: testutil.TracerPayloadWithChunk(testutil.TraceChunkWithSpan(span1)),
			Source:        want,
		})
		agnt.Process(&api.Payload{
			TracerPayload: testutil.TracerPayloadWithChunk(testutil.TraceChunkWithSpans([]*pb.Span{
				span2,
				span2,
			})),
			Source: want,
		})
		assert.EqualValues(2, want.TracesFiltered.Load())
		assert.EqualValues(3, want.SpansFiltered.Load())
	})

	t.Run("BlacklistPayload", func(t *testing.T) {
		// Regression test for DataDog/datadog-agent#6500
		cfg := config.New()
		cfg.Endpoints[0].APIKey = "test"
		cfg.Ignore["resource"] = []string{"^INSERT.*"}
		ctx, cancel := context.WithCancel(context.Background())
		agnt := NewTestAgent(ctx, cfg, telemetry.NewNoopCollector())
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
		assert.EqualValues(1, want.TracesFiltered.Load())
		assert.EqualValues(2, want.SpansFiltered.Load())
		payloads := agnt.TraceWriter.(*mockTraceWriter).payloads
		assert.NotEmpty(payloads, "no payloads were written")
		span := payloads[0].TracerPayload.Chunks[0].Spans[0]
		assert.Equal("unnamed_operation", span.Name)
	})

	t.Run("Stats/Priority", func(t *testing.T) {
		cfg := config.New()
		cfg.Endpoints[0].APIKey = "test"
		ctx, cancel := context.WithCancel(context.Background())
		agnt := NewTestAgent(ctx, cfg, telemetry.NewNoopCollector())
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
		assert.EqualValues(t, 1, want.TracesPriorityNone.Load())
		assert.EqualValues(t, 2, samplingPriorityTagValues["-1"])
		assert.EqualValues(t, 3, samplingPriorityTagValues["0"])
		assert.EqualValues(t, 4, samplingPriorityTagValues["1"])
		assert.EqualValues(t, 5, samplingPriorityTagValues["2"])
	})

	t.Run("normalizing", func(t *testing.T) {
		cfg := config.New()
		cfg.Endpoints[0].APIKey = "test"
		ctx, cancel := context.WithCancel(context.Background())
		agnt := NewTestAgent(ctx, cfg, telemetry.NewNoopCollector())
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
		agnt.Process(&api.Payload{
			TracerPayload: tp,
			Source:        agnt.Receiver.Stats.GetTagStats(info.Tags{}),
		})
		payloads := agnt.TraceWriter.(*mockTraceWriter).payloads
		assert.NotEmpty(t, payloads, "no payloads were written")
		span := payloads[0].TracerPayload.Chunks[0].Spans[0]
		assert.Equal(t, "unnamed_operation", span.Name)
		assert.Equal(t, "something_that_should_be_a_metric", span.Service)
	})

	t.Run("_dd.hostname", func(t *testing.T) {
		cfg := config.New()
		cfg.Endpoints[0].APIKey = "test"
		ctx, cancel := context.WithCancel(context.Background())
		agnt := NewTestAgent(ctx, cfg, telemetry.NewNoopCollector())
		defer cancel()

		tp := testutil.TracerPayloadWithChunk(testutil.RandomTraceChunk(1, 1))
		tp.Chunks[0].Priority = int32(sampler.PriorityUserKeep)
		tp.Chunks[0].Spans[0].Meta["_dd.hostname"] = "tracer-hostname"
		agnt.Process(&api.Payload{
			TracerPayload: tp,
			Source:        agnt.Receiver.Stats.GetTagStats(info.Tags{}),
		})
		payloads := agnt.TraceWriter.(*mockTraceWriter).payloads
		assert.NotEmpty(t, payloads, "no payloads were written")
		tp = payloads[0].TracerPayload
		assert.Equal(t, "tracer-hostname", tp.Hostname)
	})

	t.Run("aas", func(t *testing.T) {
		t.Setenv("WEBSITE_STACK", "true")
		t.Setenv("WEBSITE_APPSERVICEAPPLOGS_TRACE_ENABLED", "false")
		cfg := config.New()
		cfg.Endpoints[0].APIKey = "test"
		ctx, cancel := context.WithCancel(context.Background())
		agnt := NewAgent(ctx, cfg, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, gzip.NewComponent())
		agnt.TraceWriter = &mockTraceWriter{}
		defer cancel()

		tp := testutil.TracerPayloadWithChunk(testutil.RandomTraceChunk(1, 1))
		tp.Chunks[0].Priority = int32(sampler.PriorityUserKeep)
		agnt.Process(&api.Payload{
			TracerPayload: tp,
			Source:        agnt.Receiver.Stats.GetTagStats(info.Tags{}),
		})
		payloads := agnt.TraceWriter.(*mockTraceWriter).payloads
		assert.NotEmpty(t, payloads, "no payloads were written")
		tp = payloads[0].TracerPayload

		for _, chunk := range tp.Chunks {
			for _, span := range chunk.Spans {
				assert.Contains(t, span.Meta, "aas.resource.id")
				assert.Contains(t, span.Meta, "aas.site.name")
				assert.Contains(t, span.Meta, "aas.site.type")
			}
		}
	})

	t.Run("DiscardSpans", func(t *testing.T) {
		cfg := config.New()
		cfg.Endpoints[0].APIKey = "test"
		ctx, cancel := context.WithCancel(context.Background())
		agnt := NewTestAgent(ctx, cfg, telemetry.NewNoopCollector())
		defer cancel()

		testDiscardFunction := func(span *pb.Span) bool {
			return span.Meta["irrelevant"] == "true"
		}
		agnt.DiscardSpan = testDiscardFunction

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

		c := spansToChunk(span1, span2, span3)
		c.Priority = 1
		tp := testutil.TracerPayloadWithChunk(c)

		agnt.Process(&api.Payload{
			TracerPayload: tp,
			Source:        agnt.Receiver.Stats.GetTagStats(info.Tags{}),
		})
		payloads := agnt.TraceWriter.(*mockTraceWriter).payloads
		assert.NotEmpty(t, payloads, "no payloads were written")
		payload := payloads[0]
		assert.Equal(t, 2, int(payload.SpanCount))
		assert.NotContains(t, payload.TracerPayload.Chunks[0].Spans[0].Meta, "irrelevant")
		assert.NotContains(t, payload.TracerPayload.Chunks[0].Spans[1].Meta, "irrelevant")
	})

	t.Run("chunking", func(t *testing.T) {
		cfg := config.New()
		cfg.Endpoints[0].APIKey = "test"
		ctx, cancel := context.WithCancel(context.Background())
		agnt := NewTestAgent(ctx, cfg, telemetry.NewNoopCollector())
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
		agnt.Process(&api.Payload{
			TracerPayload: tp,
			Source:        agnt.Receiver.Stats.GetTagStats(info.Tags{}),
		})

		payloads := agnt.TraceWriter.(*mockTraceWriter).payloads
		// and expecting it to result in 3 payloads
		assert.Len(t, payloads, 3)
	})
}

func spansToChunk(spans ...*pb.Span) *pb.TraceChunk {
	return &pb.TraceChunk{Spans: spans, Tags: make(map[string]string)}
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
			cfg := config.New()
			cfg.Features[tc.features] = struct{}{}
			cfg.Endpoints[0].APIKey = "test"
			if tc.withFargate {
				cfg.FargateOrchestrator = config.OrchestratorECS
			}
			cfg.RareSamplerEnabled = true
			agent := NewTestAgent(context.TODO(), cfg, telemetry.NewNoopCollector())
			tc.in.Source = agent.Receiver.Stats.GetTagStats(info.Tags{})
			agent.Process(tc.in)
			mco := agent.Concentrator.(*mockConcentrator)

			if len(tc.expected.Traces) == 0 {
				assert.Len(t, mco.stats, 0)
				return
			}
			require.Len(t, mco.stats, 1)
			assert.Equal(t, tc.expected, mco.stats[0])

			if tc.expectedSampled != nil && len(tc.expectedSampled.Chunks) > 0 {
				payloads := agent.TraceWriter.(*mockTraceWriter).payloads
				assert.NotEmpty(t, payloads, "no payloads were written")
				assert.Equal(t, tc.expectedSampled, payloads[0].TracerPayload)
			}
		})
	}
}

func TestClientComputedTopLevel(t *testing.T) {
	cfg := config.New()
	cfg.Endpoints[0].APIKey = "test"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t.Run("onNotTop", func(t *testing.T) {
		agnt := NewTestAgent(ctx, cfg, telemetry.NewNoopCollector())
		chunk := testutil.TraceChunkWithSpan(testutil.RandomSpan())
		chunk.Priority = 2
		tp := testutil.TracerPayloadWithChunk(chunk)
		agnt.Process(&api.Payload{
			TracerPayload:          tp,
			Source:                 agnt.Receiver.Stats.GetTagStats(info.Tags{}),
			ClientComputedTopLevel: true,
		})
		payloads := agnt.TraceWriter.(*mockTraceWriter).payloads
		assert.NotEmpty(t, payloads, "no payloads were written")
		_, ok := payloads[0].TracerPayload.Chunks[0].Spans[0].Metrics["_top_level"]
		assert.False(t, ok)
	})

	t.Run("off", func(t *testing.T) {
		agnt := NewTestAgent(ctx, cfg, telemetry.NewNoopCollector())
		chunk := testutil.TraceChunkWithSpan(testutil.RandomSpan())
		chunk.Priority = 2
		tp := testutil.TracerPayloadWithChunk(chunk)
		agnt.Process(&api.Payload{
			TracerPayload:          tp,
			Source:                 agnt.Receiver.Stats.GetTagStats(info.Tags{}),
			ClientComputedTopLevel: false,
		})
		payloads := agnt.TraceWriter.(*mockTraceWriter).payloads
		assert.NotEmpty(t, payloads, "no payloads were written")
		_, ok := payloads[0].TracerPayload.Chunks[0].Spans[0].Metrics["_top_level"]
		assert.True(t, ok)
	})

	t.Run("onTop", func(t *testing.T) {
		agnt := NewTestAgent(ctx, cfg, telemetry.NewNoopCollector())
		span := testutil.RandomSpan()
		span.Metrics = map[string]float64{
			"_dd.top_level": 1,
		}
		chunk := testutil.TraceChunkWithSpan(span)
		chunk.Priority = 2
		tp := testutil.TracerPayloadWithChunk(chunk)
		agnt.Process(&api.Payload{
			TracerPayload:          tp,
			Source:                 agnt.Receiver.Stats.GetTagStats(info.Tags{}),
			ClientComputedTopLevel: true,
		})
		payloads := agnt.TraceWriter.(*mockTraceWriter).payloads
		assert.NotEmpty(t, payloads, "no payloads were written")
		_, ok := payloads[0].TracerPayload.Chunks[0].Spans[0].Metrics["_top_level"]
		assert.True(t, ok)
		_, ok = payloads[0].TracerPayload.Chunks[0].Spans[0].Metrics["_dd.top_level"]
		assert.True(t, ok)
	})
}

func TestFilteredByTags(t *testing.T) {
	for name, tt := range map[string]*struct {
		require      []*config.Tag
		reject       []*config.Tag
		requireRegex []*config.TagRegex
		rejectRegex  []*config.TagRegex
		span         pb.Span
		drop         bool
	}{
		"keep-span-with-tag-from-required-list": {
			require: []*config.Tag{{K: "key", V: "val"}},
			span:    pb.Span{Meta: map[string]string{"key": "val"}},
			drop:    false,
		},
		"keep-span-with-tag-value-diff-rejected-list": {
			reject: []*config.Tag{{K: "key", V: "val"}},
			span:   pb.Span{Meta: map[string]string{"key": "val4"}},
			drop:   false,
		},
		"keep-span-with-tag-diff-rejected-list": {
			reject: []*config.Tag{{K: "something", V: "else"}},
			span:   pb.Span{Meta: map[string]string{"key": "val"}},
			drop:   false,
		},
		"keep-span-with-tag-from-required-list-and-tag-value-diff-rejected-list": {
			require: []*config.Tag{{K: "something", V: "else"}},
			reject:  []*config.Tag{{K: "bad-key", V: "bad-value"}},
			span:    pb.Span{Meta: map[string]string{"something": "else", "bad-key": "other-value"}},
			drop:    false,
		},
		"keep-span-with-tag-from-required-list-whithout-value-and-tag-value-diff-rejected-list": {
			require: []*config.Tag{{K: "key", V: "value"}, {K: "key-only"}},
			reject:  []*config.Tag{{K: "bad-key", V: "bad-value"}},
			span:    pb.Span{Meta: map[string]string{"key": "value", "key-only": "but-also-value", "bad-key": "not-bad-value"}},
			drop:    false,
		},
		"drop-span-with-tag-value-diff-required-list": {
			require: []*config.Tag{{K: "key", V: "val"}},
			span:    pb.Span{Meta: map[string]string{"key": "val2"}},
			drop:    true,
		},
		"drop-span-with-tag-key-diff-required-list": {
			require: []*config.Tag{{K: "something", V: "else"}},
			span:    pb.Span{Meta: map[string]string{"key": "val"}},
			drop:    true,
		},
		"drop-span-with-tag-from-rejected-list-without-value": {
			require: []*config.Tag{{K: "valid"}, {K: "test"}},
			reject:  []*config.Tag{{K: "test"}},
			span:    pb.Span{Meta: map[string]string{"test": "random", "valid": "random"}},
			drop:    true,
		},
		"drop-span-with-tag-value-diff-required-list-and-tag-from-rejected-list": {
			require: []*config.Tag{{K: "valid-key", V: "valid-value"}, {K: "test"}},
			reject:  []*config.Tag{{K: "test"}},
			span:    pb.Span{Meta: map[string]string{"test": "random", "valid-key": "wrong-value"}},
			drop:    true,
		},
		"drop-span-with-tag-value-from-rejected-list": {
			reject: []*config.Tag{{K: "key", V: "val"}},
			span:   pb.Span{Meta: map[string]string{"key": "val"}},
			drop:   true,
		},
		"drop-span-with-tag-from-required-list-but-with-tag-from-rejected-list-without-value": {
			require: []*config.Tag{{K: "something", V: "else"}, {K: "key-only"}},
			reject:  []*config.Tag{{K: "bad-key", V: "bad-value"}, {K: "bad-key-only"}},
			span:    pb.Span{Meta: map[string]string{"something": "else", "key-only": "but-also-value", "bad-key-only": "random"}},
			drop:    true,
		},
		"drop-span-with-tag-from-rejected-regexp-list": {
			require:      []*config.Tag{{K: "key", V: "valid"}},
			requireRegex: []*config.TagRegex{{K: "something", V: regexp.MustCompile("^else[0-9]{1}$")}},
			span:         pb.Span{Meta: map[string]string{"key": "valid", "something": "else11"}},
			drop:         true,
		},
		"drop-span-with-tag-from-rejected-regexp-list-but-without-tag-from-rejected-list": {
			reject:      []*config.Tag{{K: "test", V: "bad"}},
			rejectRegex: []*config.TagRegex{{K: "bad-key", V: regexp.MustCompile("^bad-value$")}},
			span:        pb.Span{Meta: map[string]string{"bad-key": "bad-value"}},
			drop:        true,
		},
		"keep-span-with-tag-from-required-regexp-list": {
			requireRegex: []*config.TagRegex{{K: "key", V: regexp.MustCompile("^val[0-9]{1}$")}},
			span:         pb.Span{Meta: map[string]string{"key": "val1"}},
			drop:         false,
		},
		"keep-span-with-tag-value-diff-rejected-regexp-list": {
			rejectRegex: []*config.TagRegex{{K: "key", V: regexp.MustCompile("^val$")}},
			span:        pb.Span{Meta: map[string]string{"key": "val4"}},
			drop:        false,
		},
		"keep-span-with-tag-key-value-diff-rejected-regexp-list": {
			rejectRegex: []*config.TagRegex{{K: "something", V: regexp.MustCompile("^else$")}},
			span:        pb.Span{Meta: map[string]string{"key": "val"}},
			drop:        false,
		},
		"keep-span-with-tag-from-required-regexp-list-and-tag-value-diff-rejected-regexp-list": {
			requireRegex: []*config.TagRegex{{K: "something", V: regexp.MustCompile("^else$")}},
			rejectRegex:  []*config.TagRegex{{K: "bad-key", V: regexp.MustCompile("^bad-value$")}},
			span:         pb.Span{Meta: map[string]string{"something": "else", "bad-key": "other-value"}},
			drop:         false,
		},
		"keep-span-with-tag-from-required-regexp-list-without-value-and-tag-value-diff-rejected-regexp-list": {
			requireRegex: []*config.TagRegex{{K: "key", V: regexp.MustCompile("^value$")}, {K: "key-only"}},
			rejectRegex:  []*config.TagRegex{{K: "bad-key", V: regexp.MustCompile("^bad-value$")}},
			span:         pb.Span{Meta: map[string]string{"key": "value", "key-only": "but-also-value", "bad-key": "not-bad-value"}},
			drop:         false,
		},
		"drop-span-with-tag-value-diff-required-regexp-list": {
			requireRegex: []*config.TagRegex{{K: "key", V: regexp.MustCompile("^val$")}},
			span:         pb.Span{Meta: map[string]string{"key": "val2"}},
			drop:         true,
		},
		"drop-span-with-tag-key-diff-required-regexp-list": {
			requireRegex: []*config.TagRegex{{K: "something", V: regexp.MustCompile("^else$")}},
			span:         pb.Span{Meta: map[string]string{"key": "val"}},
			drop:         true,
		},
		"drop-span-with-tag-from-rejected-regexp-list-without-value": {
			requireRegex: []*config.TagRegex{{K: "valid"}, {K: "test"}},
			rejectRegex:  []*config.TagRegex{{K: "test"}},
			span:         pb.Span{Meta: map[string]string{"test": "random", "valid": "random"}},
			drop:         true,
		},
		"drop-span-with-tag-value-diff-required-regexp-list-and-tag-from-required-regexp-list-without-value": {
			requireRegex: []*config.TagRegex{{K: "valid-key", V: regexp.MustCompile("^valid-value$")}, {K: "test"}},
			rejectRegex:  []*config.TagRegex{{K: "test"}},
			span:         pb.Span{Meta: map[string]string{"test": "random", "valid-key": "wrong-value"}},
			drop:         true,
		},
		"drop-span-with-tag-from-rejected-regexp-list-and-without-required-regexp-list": {
			rejectRegex: []*config.TagRegex{{K: "key", V: regexp.MustCompile("^val$")}},
			span:        pb.Span{Meta: map[string]string{"key": "val"}},
			drop:        true,
		},
		"drop-span-with-tag-from-required-regexp-list-but-with-tag-from-rejected-regexp-list-without-value": {
			requireRegex: []*config.TagRegex{{K: "something", V: regexp.MustCompile("^else$")}, {K: "key-only"}},
			rejectRegex:  []*config.TagRegex{{K: "bad-key", V: regexp.MustCompile("^bad-value$")}, {K: "bad-key-only"}},
			span:         pb.Span{Meta: map[string]string{"something": "else", "key-only": "but-also-value", "bad-key-only": "random"}},
			drop:         true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			if filteredByTags(&tt.span, tt.require, tt.reject, tt.requireRegex, tt.rejectRegex) != tt.drop {
				t.Fatal()
			}
		})
	}
}

func BenchmarkFilteredByTags(b *testing.B) {
	type FilteredByTagTestData struct {
		require      []*config.Tag
		reject       []*config.Tag
		requireRegex []*config.TagRegex
		rejectRegex  []*config.TagRegex
		span         pb.Span
	}

	b.Run("FilteredByTags", func(b *testing.B) {
		tt := FilteredByTagTestData{
			require: []*config.Tag{{K: "key1", V: "val1"}, {K: "key2", V: "val2"}},
			reject:  []*config.Tag{{K: "key3", V: "val3"}},
			span:    pb.Span{Meta: map[string]string{"key1": "val1", "key2": "val2"}},
		}

		runTraceFilteringBenchmark(b, &tt.span, tt.require, tt.reject, tt.requireRegex, tt.rejectRegex)
	})

	b.Run("FilteredByRegexTags", func(b *testing.B) {
		tt := FilteredByTagTestData{
			requireRegex: []*config.TagRegex{{K: "key1", V: regexp.MustCompile("^val1$")}, {K: "key2", V: regexp.MustCompile("^val2$")}},
			rejectRegex:  []*config.TagRegex{{K: "key3", V: regexp.MustCompile("^val3$")}},
			span:         pb.Span{Meta: map[string]string{"key1": "val1", "key2": "val2"}},
		}

		runTraceFilteringBenchmark(b, &tt.span, tt.require, tt.reject, tt.requireRegex, tt.rejectRegex)
	})
}

func runTraceFilteringBenchmark(b *testing.B, root *pb.Span, require []*config.Tag, reject []*config.Tag, requireRegex []*config.TagRegex, rejectRegex []*config.TagRegex) {
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		filteredByTags(root, require, reject, requireRegex, rejectRegex)
	}
}

func TestClientComputedStats(t *testing.T) {
	cfg := config.New()
	cfg.Endpoints[0].APIKey = "test"
	ctx, cancel := context.WithCancel(context.Background())
	agnt := NewTestAgent(ctx, cfg, telemetry.NewNoopCollector())
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
		mco := agnt.Concentrator.(*mockConcentrator)
		assert.Len(t, mco.stats, 0)
	})

	t.Run("off", func(t *testing.T) {
		agnt.Process(&api.Payload{
			TracerPayload:       tp,
			Source:              agnt.Receiver.Stats.GetTagStats(info.Tags{}),
			ClientComputedStats: false,
		})
		mco := agnt.Concentrator.(*mockConcentrator)
		assert.Len(t, mco.stats, 1)
	})
}

func TestSampling(t *testing.T) {
	// agentConfig allows the test to customize how the agent is configured.
	type agentConfig struct {
		rareSamplerDisabled, errorsSampled, noPrioritySampled, probabilisticSampler bool
		probabilisticSamplerSamplingPercentage                                      float32
	}
	// configureAgent creates a new agent using the provided configuration.
	configureAgent := func(ac agentConfig, statsd statsd.ClientInterface) *Agent {
		cfg := &config.AgentConfig{
			RareSamplerEnabled:                     !ac.rareSamplerDisabled,
			RareSamplerCardinality:                 200,
			RareSamplerTPS:                         5,
			RareSamplerCooldownPeriod:              5 * time.Minute,
			ProbabilisticSamplerEnabled:            ac.probabilisticSampler,
			ProbabilisticSamplerSamplingPercentage: ac.probabilisticSamplerSamplingPercentage,
		}
		sampledCfg := &config.AgentConfig{
			ExtraSampleRate:    1,
			TargetTPS:          5,
			ErrorTPS:           10,
			RareSamplerEnabled: !ac.rareSamplerDisabled,
		}

		a := &Agent{
			NoPrioritySampler:    sampler.NewNoPrioritySampler(cfg),
			ErrorsSampler:        sampler.NewErrorsSampler(cfg),
			PrioritySampler:      sampler.NewPrioritySampler(cfg, &sampler.DynamicConfig{}),
			RareSampler:          sampler.NewRareSampler(cfg),
			ProbabilisticSampler: sampler.NewProbabilisticSampler(cfg),
			SamplerMetrics:       sampler.NewMetrics(statsd),
			conf:                 cfg,
		}
		a.SamplerMetrics.Add(a.NoPrioritySampler, a.ErrorsSampler, a.PrioritySampler, a.RareSampler)
		if ac.errorsSampled {
			a.ErrorsSampler = sampler.NewErrorsSampler(sampledCfg)
		}
		if ac.noPrioritySampled {
			a.NoPrioritySampler = sampler.NewNoPrioritySampler(sampledCfg)
		}
		return a
	}
	// generateProcessedTrace creates a new dummy trace to send to the samplers.
	generateProcessedTrace := func(p sampler.SamplingPriority, hasErrors bool) traceutil.ProcessedTrace {
		root := &pb.Span{
			Service:  "serv1",
			Start:    time.Now().UnixNano(),
			Duration: (100 * time.Millisecond).Nanoseconds(),
			Metrics:  map[string]float64{"_top_level": 1},
		}
		if hasErrors {
			root.Error = 1
		}
		pt := traceutil.ProcessedTrace{TraceChunk: testutil.TraceChunkWithSpan(root), Root: root}
		pt.TraceChunk.Priority = int32(p)
		pt.TracerEnv = "test-env"
		return pt
	}
	type samplingTestCase struct {
		trace        traceutil.ProcessedTrace
		wantSampled  bool
		expectStatsd func(statsdClient *mockStatsd.MockClientInterface)
	}

	for name, tt := range map[string]struct {
		agentConfig agentConfig
		testCases   []samplingTestCase
	}{
		"nopriority-unsampled": {
			agentConfig: agentConfig{noPrioritySampled: false, rareSamplerDisabled: true},
			testCases: []samplingTestCase{
				{
					trace:       generateProcessedTrace(sampler.PriorityNone, false),
					wantSampled: false,
					expectStatsd: func(statsdClient *mockStatsd.MockClientInterface) {
						statsdClient.EXPECT().Count(sampler.MetricSamplerKept, gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
						statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(1), []string{"sampler:no_priority", "target_service:serv1", "target_env:test-env"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(1), []string{"sampler:no_priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:error"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareHits, int64(0), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareMisses, int64(0), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricsRareShrinks, float64(0), nil, gomock.Any()).Times(1)
					},
				},
			},
		},
		"nopriority-sampled": {
			agentConfig: agentConfig{noPrioritySampled: true},
			testCases: []samplingTestCase{
				{
					trace:       generateProcessedTrace(sampler.PriorityNone, false),
					wantSampled: true,
					expectStatsd: func(statsdClient *mockStatsd.MockClientInterface) {
						statsdClient.EXPECT().Count(sampler.MetricSamplerKept, int64(1), []string{"sampler:rare", "target_service:serv1", "target_env:test-env"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(1), []string{"sampler:rare", "target_service:serv1", "target_env:test-env"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:no_priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:error"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareHits, int64(1), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareMisses, int64(0), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricsRareShrinks, float64(0), nil, gomock.Any()).Times(1)
					},
				},
			},
		},
		"prio-unsampled": {
			agentConfig: agentConfig{rareSamplerDisabled: true},
			testCases: []samplingTestCase{
				{
					trace:       generateProcessedTrace(sampler.PriorityAutoDrop, false),
					wantSampled: false,
					expectStatsd: func(statsdClient *mockStatsd.MockClientInterface) {
						statsdClient.EXPECT().Count(sampler.MetricSamplerKept, gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
						statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(1), []string{"sampler:priority", "sampling_priority:auto_drop", "target_service:serv1", "target_env:test-env"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:no_priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:error"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(1), []string{"sampler:priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareHits, int64(0), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareMisses, int64(0), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricsRareShrinks, float64(0), nil, gomock.Any()).Times(1)
					},
				},
			},
		},
		"prio-sampled": {
			agentConfig: agentConfig{},
			testCases: []samplingTestCase{
				{
					trace:       generateProcessedTrace(sampler.PriorityAutoKeep, false),
					wantSampled: true,
					expectStatsd: func(statsdClient *mockStatsd.MockClientInterface) {
						statsdClient.EXPECT().Count(sampler.MetricSamplerKept, int64(1), []string{"sampler:rare", "target_service:serv1", "target_env:test-env"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(1), []string{"sampler:rare", "target_service:serv1", "target_env:test-env"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:no_priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:error"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareHits, int64(1), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareMisses, int64(0), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricsRareShrinks, float64(0), nil, gomock.Any()).Times(1)
					},
				},
			},
		},
		"error-unsampled": {
			agentConfig: agentConfig{errorsSampled: false, rareSamplerDisabled: true},
			testCases: []samplingTestCase{
				{
					trace:       generateProcessedTrace(sampler.PriorityNone, true),
					wantSampled: false,
					expectStatsd: func(statsdClient *mockStatsd.MockClientInterface) {
						statsdClient.EXPECT().Count(sampler.MetricSamplerKept, gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
						statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(1), []string{"sampler:error", "target_service:serv1", "target_env:test-env"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(1), []string{"sampler:no_priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:error"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareHits, int64(0), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareMisses, int64(0), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricsRareShrinks, float64(0), nil, gomock.Any()).Times(1)
					},
				},
			},
		},
		"error-sampled": {
			agentConfig: agentConfig{errorsSampled: true},
			testCases: []samplingTestCase{
				{
					trace:       generateProcessedTrace(sampler.PriorityNone, true),
					wantSampled: true,
					expectStatsd: func(statsdClient *mockStatsd.MockClientInterface) {
						statsdClient.EXPECT().Count(sampler.MetricSamplerKept, int64(1), []string{"sampler:rare", "target_service:serv1", "target_env:test-env"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(1), []string{"sampler:rare", "target_service:serv1", "target_env:test-env"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:no_priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:error"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareHits, int64(1), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareMisses, int64(0), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricsRareShrinks, float64(0), nil, gomock.Any()).Times(1)
					},
				},
			},
		},
		"error-sampled-prio-unsampled": {
			agentConfig: agentConfig{errorsSampled: true},
			testCases: []samplingTestCase{
				{
					trace:       generateProcessedTrace(sampler.PriorityAutoDrop, true),
					wantSampled: true,
					expectStatsd: func(statsdClient *mockStatsd.MockClientInterface) {
						statsdClient.EXPECT().Count(sampler.MetricSamplerKept, int64(1), []string{"sampler:rare", "target_service:serv1", "target_env:test-env"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(1), []string{"sampler:rare", "target_service:serv1", "target_env:test-env"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:no_priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:error"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareHits, int64(1), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareMisses, int64(0), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricsRareShrinks, float64(0), nil, gomock.Any()).Times(1)
					},
				},
			},
		},
		"error-sampled-prio-sampled": {
			agentConfig: agentConfig{errorsSampled: false},
			testCases: []samplingTestCase{
				{
					trace:       generateProcessedTrace(sampler.PriorityAutoKeep, true),
					wantSampled: true,
					expectStatsd: func(statsdClient *mockStatsd.MockClientInterface) {
						statsdClient.EXPECT().Count(sampler.MetricSamplerKept, int64(1), []string{"sampler:rare", "target_service:serv1", "target_env:test-env"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(1), []string{"sampler:rare", "target_service:serv1", "target_env:test-env"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:no_priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:error"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareHits, int64(1), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareMisses, int64(0), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricsRareShrinks, float64(0), nil, gomock.Any()).Times(1)
					},
				},
			},
		},
		"error-prio-sampled": {
			agentConfig: agentConfig{errorsSampled: true},
			testCases: []samplingTestCase{
				{
					trace:       generateProcessedTrace(sampler.PriorityAutoKeep, true),
					wantSampled: true,
					expectStatsd: func(statsdClient *mockStatsd.MockClientInterface) {
						statsdClient.EXPECT().Count(sampler.MetricSamplerKept, int64(1), []string{"sampler:rare", "target_service:serv1", "target_env:test-env"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(1), []string{"sampler:rare", "target_service:serv1", "target_env:test-env"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:no_priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:error"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareHits, int64(1), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareMisses, int64(0), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricsRareShrinks, float64(0), nil, gomock.Any()).Times(1)
					},
				},
			},
		},
		"error-prio-unsampled": {
			agentConfig: agentConfig{errorsSampled: false, rareSamplerDisabled: true},
			testCases: []samplingTestCase{
				{
					trace:       generateProcessedTrace(sampler.PriorityAutoDrop, true),
					wantSampled: false,
					expectStatsd: func(statsdClient *mockStatsd.MockClientInterface) {
						statsdClient.EXPECT().Count(sampler.MetricSamplerKept, gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
						statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(1), []string{"sampler:error", "target_service:serv1", "target_env:test-env"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:no_priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:error"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(1), []string{"sampler:priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareHits, int64(0), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareMisses, int64(0), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricsRareShrinks, float64(0), nil, gomock.Any()).Times(1)
					},
				},
			},
		},
		"rare-sampler-catch-unsampled": {
			agentConfig: agentConfig{},
			testCases: []samplingTestCase{
				{
					trace:       generateProcessedTrace(sampler.PriorityAutoDrop, false),
					wantSampled: true,
					expectStatsd: func(statsdClient *mockStatsd.MockClientInterface) {
						statsdClient.EXPECT().Count(sampler.MetricSamplerKept, int64(1), []string{"sampler:rare", "target_service:serv1", "target_env:test-env"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(1), []string{"sampler:rare", "target_service:serv1", "target_env:test-env"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:no_priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:error"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareHits, int64(1), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareMisses, int64(0), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricsRareShrinks, float64(0), nil, gomock.Any()).Times(1)
					},
				},
			},
		},
		"rare-sampler-catch-sampled": {
			agentConfig: agentConfig{},
			testCases: []samplingTestCase{
				{
					trace:       generateProcessedTrace(sampler.PriorityAutoKeep, false),
					wantSampled: true,
					expectStatsd: func(statsdClient *mockStatsd.MockClientInterface) {
						statsdClient.EXPECT().Count(sampler.MetricSamplerKept, int64(1), []string{"sampler:rare", "target_service:serv1", "target_env:test-env"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(1), []string{"sampler:rare", "target_service:serv1", "target_env:test-env"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:no_priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:error"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareHits, int64(1), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareMisses, int64(0), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricsRareShrinks, float64(0), nil, gomock.Any()).Times(1)
					},
				},
				{
					trace:       generateProcessedTrace(sampler.PriorityAutoDrop, false),
					wantSampled: false,
					expectStatsd: func(statsdClient *mockStatsd.MockClientInterface) {
						statsdClient.EXPECT().Count(sampler.MetricSamplerKept, gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
						statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(1), []string{"sampler:priority", "sampling_priority:auto_drop", "target_service:serv1", "target_env:test-env"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:no_priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:error"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(1), []string{"sampler:priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareHits, int64(0), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareMisses, int64(0), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricsRareShrinks, float64(0), nil, gomock.Any()).Times(1)
					},
				},
			},
		},
		"rare-sampler-disabled": {
			agentConfig: agentConfig{rareSamplerDisabled: true},
			testCases: []samplingTestCase{
				{
					trace:       generateProcessedTrace(sampler.PriorityAutoDrop, false),
					wantSampled: false,
					expectStatsd: func(statsdClient *mockStatsd.MockClientInterface) {
						statsdClient.EXPECT().Count(sampler.MetricSamplerKept, gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
						statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(1), []string{"sampler:priority", "sampling_priority:auto_drop", "target_service:serv1", "target_env:test-env"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:no_priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:error"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(1), []string{"sampler:priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareHits, int64(0), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareMisses, int64(0), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricsRareShrinks, float64(0), nil, gomock.Any()).Times(1)
					},
				},
			},
		},
		// These tests use 0% and 100% to ensure traces are sampled or not by the sampler. They are
		// intended to test the sampling logic of the agent under various configurations. The exact
		// behavior of the probabilistic sampler is tested in pkg/trace/sampler.
		"probabilistic-no-prio-100": {
			agentConfig: agentConfig{noPrioritySampled: false, rareSamplerDisabled: true, probabilisticSampler: true, probabilisticSamplerSamplingPercentage: 100},
			testCases: []samplingTestCase{
				{
					trace:       generateProcessedTrace(sampler.PriorityNone, false),
					wantSampled: true,
					expectStatsd: func(statsdClient *mockStatsd.MockClientInterface) {
						statsdClient.EXPECT().Count(sampler.MetricSamplerKept, int64(1), []string{"sampler:probabilistic", "target_service:serv1"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(1), []string{"sampler:probabilistic", "target_service:serv1"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:no_priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:error"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareHits, int64(0), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareMisses, int64(0), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricsRareShrinks, float64(0), nil, gomock.Any()).Times(1)
					},
				},
			},
		},
		"probabilistic-no-prio-0": {
			agentConfig: agentConfig{noPrioritySampled: false, rareSamplerDisabled: true, probabilisticSampler: true, probabilisticSamplerSamplingPercentage: 0},
			testCases: []samplingTestCase{
				{
					trace:       generateProcessedTrace(sampler.PriorityNone, false),
					wantSampled: false,
					expectStatsd: func(statsdClient *mockStatsd.MockClientInterface) {
						statsdClient.EXPECT().Count(sampler.MetricSamplerKept, gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
						statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(1), []string{"sampler:probabilistic", "target_service:serv1"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:no_priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:error"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareHits, int64(0), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareMisses, int64(0), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricsRareShrinks, float64(0), nil, gomock.Any()).Times(1)
					},
				},
			},
		},
		"probabilistic-prio-drop-100": {
			agentConfig: agentConfig{noPrioritySampled: false, rareSamplerDisabled: true, probabilisticSampler: true, probabilisticSamplerSamplingPercentage: 100},
			testCases: []samplingTestCase{
				{
					trace:       generateProcessedTrace(sampler.PriorityUserDrop, false),
					wantSampled: true,
					expectStatsd: func(statsdClient *mockStatsd.MockClientInterface) {
						statsdClient.EXPECT().Count(sampler.MetricSamplerKept, int64(1), []string{"sampler:probabilistic", "target_service:serv1"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(1), []string{"sampler:probabilistic", "target_service:serv1"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:no_priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:error"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareHits, int64(0), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareMisses, int64(0), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricsRareShrinks, float64(0), nil, gomock.Any()).Times(1)
					},
				},
			},
		},
		"probabilistic-prio-drop-0": {
			agentConfig: agentConfig{noPrioritySampled: false, rareSamplerDisabled: true, probabilisticSampler: true, probabilisticSamplerSamplingPercentage: 0},
			testCases: []samplingTestCase{
				{
					trace:       generateProcessedTrace(sampler.PriorityUserDrop, false),
					wantSampled: false,
					expectStatsd: func(statsdClient *mockStatsd.MockClientInterface) {
						statsdClient.EXPECT().Count(sampler.MetricSamplerKept, gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
						statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(1), []string{"sampler:probabilistic", "target_service:serv1"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:no_priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:error"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareHits, int64(0), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareMisses, int64(0), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricsRareShrinks, float64(0), nil, gomock.Any()).Times(1)
					},
				},
			},
		},
		"probabilistic-prio-keep-100": {
			agentConfig: agentConfig{noPrioritySampled: false, rareSamplerDisabled: true, probabilisticSampler: true, probabilisticSamplerSamplingPercentage: 100},
			testCases: []samplingTestCase{
				{
					trace:       generateProcessedTrace(sampler.PriorityUserKeep, false),
					wantSampled: true,
					expectStatsd: func(statsdClient *mockStatsd.MockClientInterface) {
						statsdClient.EXPECT().Count(sampler.MetricSamplerKept, int64(1), []string{"sampler:probabilistic", "target_service:serv1"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(1), []string{"sampler:probabilistic", "target_service:serv1"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:no_priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:error"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareHits, int64(0), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareMisses, int64(0), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricsRareShrinks, float64(0), nil, gomock.Any()).Times(1)
					},
				},
			},
		},
		"probabilistic-prio-keep-0": {
			agentConfig: agentConfig{noPrioritySampled: false, rareSamplerDisabled: true, probabilisticSampler: true, probabilisticSamplerSamplingPercentage: 0},
			testCases: []samplingTestCase{
				{
					trace:       generateProcessedTrace(sampler.PriorityUserKeep, false),
					wantSampled: false,
					expectStatsd: func(statsdClient *mockStatsd.MockClientInterface) {
						statsdClient.EXPECT().Count(sampler.MetricSamplerKept, gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
						statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(1), []string{"sampler:probabilistic", "target_service:serv1"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:no_priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:error"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareHits, int64(0), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareMisses, int64(0), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricsRareShrinks, float64(0), nil, gomock.Any()).Times(1)
					},
				},
			},
		},
		"probabilistic-rare-100": {
			agentConfig: agentConfig{noPrioritySampled: false, rareSamplerDisabled: false, probabilisticSampler: true, probabilisticSamplerSamplingPercentage: 100},
			testCases: []samplingTestCase{
				{
					trace:       generateProcessedTrace(sampler.PriorityNone, false),
					wantSampled: true,
					expectStatsd: func(statsdClient *mockStatsd.MockClientInterface) {
						statsdClient.EXPECT().Count(sampler.MetricSamplerKept, int64(1), []string{"sampler:rare", "target_service:serv1", "target_env:test-env"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(1), []string{"sampler:rare", "target_service:serv1", "target_env:test-env"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:no_priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:error"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareHits, int64(1), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareMisses, int64(0), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricsRareShrinks, float64(0), nil, gomock.Any()).Times(1)
					},
				},
			},
		},
		"probabilistic-rare-0": {
			agentConfig: agentConfig{noPrioritySampled: false, rareSamplerDisabled: false, probabilisticSampler: true, probabilisticSamplerSamplingPercentage: 0},
			testCases: []samplingTestCase{
				{
					trace:       generateProcessedTrace(sampler.PriorityNone, false),
					wantSampled: true,
					expectStatsd: func(statsdClient *mockStatsd.MockClientInterface) {
						statsdClient.EXPECT().Count(sampler.MetricSamplerKept, int64(1), []string{"sampler:rare", "target_service:serv1", "target_env:test-env"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(1), []string{"sampler:rare", "target_service:serv1", "target_env:test-env"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:no_priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:error"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareHits, int64(1), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareMisses, int64(0), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricsRareShrinks, float64(0), nil, gomock.Any()).Times(1)
					},
				},
			},
		},
		"probabilistic-rare-prio-0": {
			agentConfig: agentConfig{noPrioritySampled: false, rareSamplerDisabled: false, probabilisticSampler: true, probabilisticSamplerSamplingPercentage: 0},
			testCases: []samplingTestCase{
				{
					trace:       generateProcessedTrace(sampler.PriorityUserKeep, false),
					wantSampled: true,
					expectStatsd: func(statsdClient *mockStatsd.MockClientInterface) {
						statsdClient.EXPECT().Count(sampler.MetricSamplerKept, int64(1), []string{"sampler:rare", "target_service:serv1", "target_env:test-env"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(1), []string{"sampler:rare", "target_service:serv1", "target_env:test-env"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:no_priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:error"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareHits, int64(1), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareMisses, int64(0), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricsRareShrinks, float64(0), nil, gomock.Any()).Times(1)
					},
				},
			},
		},
		"probabilistic-error-100": {
			agentConfig: agentConfig{noPrioritySampled: false, rareSamplerDisabled: true, probabilisticSampler: true, errorsSampled: true, probabilisticSamplerSamplingPercentage: 100},
			testCases: []samplingTestCase{
				{
					trace:       generateProcessedTrace(sampler.PriorityAutoDrop, true),
					wantSampled: true,
					expectStatsd: func(statsdClient *mockStatsd.MockClientInterface) {
						statsdClient.EXPECT().Count(sampler.MetricSamplerKept, int64(1), []string{"sampler:probabilistic", "target_service:serv1"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(1), []string{"sampler:probabilistic", "target_service:serv1"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:no_priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:error"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareHits, int64(0), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareMisses, int64(0), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricsRareShrinks, float64(0), nil, gomock.Any()).Times(1)
					},
				},
			},
		},
		"probabilistic-error-0": {
			agentConfig: agentConfig{noPrioritySampled: false, rareSamplerDisabled: true, probabilisticSampler: true, errorsSampled: true, probabilisticSamplerSamplingPercentage: 0},
			testCases: []samplingTestCase{
				{
					trace:       generateProcessedTrace(sampler.PriorityAutoDrop, true),
					wantSampled: true,
					expectStatsd: func(statsdClient *mockStatsd.MockClientInterface) {
						statsdClient.EXPECT().Count(sampler.MetricSamplerKept, int64(1), []string{"sampler:error", "target_service:serv1", "target_env:test-env"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(1), []string{"sampler:error", "target_service:serv1", "target_env:test-env"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:no_priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:error"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:priority"}, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareHits, int64(0), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Count(sampler.MetricsRareMisses, int64(0), nil, gomock.Any()).Times(1)
						statsdClient.EXPECT().Gauge(sampler.MetricsRareShrinks, float64(0), nil, gomock.Any()).Times(1)
					},
				},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			statsdClient := mockStatsd.NewMockClientInterface(ctrl)
			a := configureAgent(tt.agentConfig, statsdClient)
			for _, tc := range tt.testCases {
				sampled, _ := a.traceSampling(time.Now(), &info.TagStats{}, &tc.trace)
				assert.EqualValues(t, tc.wantSampled, sampled)
				require.NotNil(t, tc.expectStatsd)
				tc.expectStatsd(statsdClient)
				a.SamplerMetrics.Report()
			}
		})
	}
}

func TestSampleTrace(t *testing.T) {
	now := time.Now()
	cfg := &config.AgentConfig{TargetTPS: 5, ErrorTPS: 1000, Features: make(map[string]struct{})}
	genSpan := func(decisionMaker string, priority sampler.SamplingPriority, err int32) traceutil.ProcessedTrace {
		root := &pb.Span{
			Service:  "serv1",
			Start:    now.UnixNano(),
			Duration: (100 * time.Millisecond).Nanoseconds(),
			Metrics:  map[string]float64{"_top_level": 1},
			Error:    err, // If 1, the Error Sampler will keep the trace, if 0, it will not be sampled
			Meta:     map[string]string{},
		}
		chunk := testutil.TraceChunkWithSpan(root)
		if decisionMaker != "" {
			chunk.Tags["_dd.p.dm"] = decisionMaker
			chunk.GetSpans()[0].Meta["_dd.p.dm"] = decisionMaker
		}
		pt := traceutil.ProcessedTrace{TraceChunk: chunk, Root: root}
		pt.TraceChunk.Priority = int32(priority)
		return pt
	}
	tests := map[string]struct {
		trace                   traceutil.ProcessedTrace
		keep                    bool
		keepWithFeature         bool
		expectStatsd            func(statsdClient *mockStatsd.MockClientInterface)
		expectStatsdWithFeature func(statsdClient *mockStatsd.MockClientInterface)
	}{
		"userdrop-error-no-dm-sampled": {
			trace:           genSpan("", sampler.PriorityUserDrop, 1),
			keep:            false,
			keepWithFeature: true,
			expectStatsd: func(statsdClient *mockStatsd.MockClientInterface) {
				statsdClient.EXPECT().Count(sampler.MetricSamplerKept, gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(1), []string{"sampler:priority", "sampling_priority:manual_drop", "target_service:serv1"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:no_priority"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:error"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:priority"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Count(sampler.MetricsRareHits, int64(0), nil, gomock.Any()).Times(1)
				statsdClient.EXPECT().Count(sampler.MetricsRareMisses, int64(0), nil, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricsRareShrinks, float64(0), nil, gomock.Any()).Times(1)
			},
			expectStatsdWithFeature: func(statsdClient *mockStatsd.MockClientInterface) {
				statsdClient.EXPECT().Count(sampler.MetricSamplerKept, int64(1), []string{"sampler:error", "target_service:serv1"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(1), []string{"sampler:error", "target_service:serv1"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:no_priority"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(1), []string{"sampler:error"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:priority"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Count(sampler.MetricsRareHits, int64(0), nil, gomock.Any()).Times(1)
				statsdClient.EXPECT().Count(sampler.MetricsRareMisses, int64(0), nil, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricsRareShrinks, float64(0), nil, gomock.Any()).Times(1)
			},
		},
		"userdrop-error-manual-dm-unsampled": {
			trace:           genSpan("-4", sampler.PriorityUserDrop, 1),
			keep:            false,
			keepWithFeature: false,
			expectStatsd: func(statsdClient *mockStatsd.MockClientInterface) {
				statsdClient.EXPECT().Count(sampler.MetricSamplerKept, gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(1), []string{"sampler:priority", "sampling_priority:manual_drop", "target_service:serv1"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:no_priority"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:error"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:priority"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Count(sampler.MetricsRareHits, int64(0), nil, gomock.Any()).Times(1)
				statsdClient.EXPECT().Count(sampler.MetricsRareMisses, int64(0), nil, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricsRareShrinks, float64(0), nil, gomock.Any()).Times(1)
			},
			expectStatsdWithFeature: func(statsdClient *mockStatsd.MockClientInterface) {
				statsdClient.EXPECT().Count(sampler.MetricSamplerKept, gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(1), []string{"sampler:priority", "sampling_priority:manual_drop", "target_service:serv1"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:no_priority"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:error"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:priority"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Count(sampler.MetricsRareHits, int64(0), nil, gomock.Any()).Times(1)
				statsdClient.EXPECT().Count(sampler.MetricsRareMisses, int64(0), nil, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricsRareShrinks, float64(0), nil, gomock.Any()).Times(1)
			},
		},
		"userdrop-error-agent-dm-sampled": {
			trace:           genSpan("-1", sampler.PriorityUserDrop, 1),
			keep:            false,
			keepWithFeature: true,
			expectStatsd: func(statsdClient *mockStatsd.MockClientInterface) {
				statsdClient.EXPECT().Count(sampler.MetricSamplerKept, gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(1), []string{"sampler:priority", "sampling_priority:manual_drop", "target_service:serv1"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:no_priority"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:error"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:priority"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Count(sampler.MetricsRareHits, int64(0), nil, gomock.Any()).Times(1)
				statsdClient.EXPECT().Count(sampler.MetricsRareMisses, int64(0), nil, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricsRareShrinks, float64(0), nil, gomock.Any()).Times(1)
			},
			expectStatsdWithFeature: func(statsdClient *mockStatsd.MockClientInterface) {
				statsdClient.EXPECT().Count(sampler.MetricSamplerKept, int64(1), []string{"sampler:error", "target_service:serv1"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(1), []string{"sampler:error", "target_service:serv1"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:no_priority"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(1), []string{"sampler:error"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:priority"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Count(sampler.MetricsRareHits, int64(0), nil, gomock.Any()).Times(1)
				statsdClient.EXPECT().Count(sampler.MetricsRareMisses, int64(0), nil, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricsRareShrinks, float64(0), nil, gomock.Any()).Times(1)
			},
		},
		"userkeep-error-no-dm-sampled": {
			trace:           genSpan("", sampler.PriorityUserKeep, 1),
			keep:            true,
			keepWithFeature: true,
			expectStatsd: func(statsdClient *mockStatsd.MockClientInterface) {
				statsdClient.EXPECT().Count(sampler.MetricSamplerKept, int64(1), []string{"sampler:priority", "sampling_priority:manual_keep", "target_service:serv1"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(1), []string{"sampler:priority", "sampling_priority:manual_keep", "target_service:serv1"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:no_priority"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:error"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:priority"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Count(sampler.MetricsRareHits, int64(0), nil, gomock.Any()).Times(1)
				statsdClient.EXPECT().Count(sampler.MetricsRareMisses, int64(0), nil, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricsRareShrinks, float64(0), nil, gomock.Any()).Times(1)
			},
			expectStatsdWithFeature: func(statsdClient *mockStatsd.MockClientInterface) {
				statsdClient.EXPECT().Count(sampler.MetricSamplerKept, int64(1), []string{"sampler:priority", "sampling_priority:manual_keep", "target_service:serv1"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(1), []string{"sampler:priority", "sampling_priority:manual_keep", "target_service:serv1"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:no_priority"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:error"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:priority"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Count(sampler.MetricsRareHits, int64(0), nil, gomock.Any()).Times(1)
				statsdClient.EXPECT().Count(sampler.MetricsRareMisses, int64(0), nil, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricsRareShrinks, float64(0), nil, gomock.Any()).Times(1)
			},
		},
		"userkeep-error-agent-dm-sampled": {
			trace:           genSpan("-1", sampler.PriorityUserKeep, 1),
			keep:            true,
			keepWithFeature: true,
			expectStatsd: func(statsdClient *mockStatsd.MockClientInterface) {
				statsdClient.EXPECT().Count(sampler.MetricSamplerKept, int64(1), []string{"sampler:priority", "sampling_priority:manual_keep", "target_service:serv1"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(1), []string{"sampler:priority", "sampling_priority:manual_keep", "target_service:serv1"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:no_priority"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:error"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:priority"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Count(sampler.MetricsRareHits, int64(0), nil, gomock.Any()).Times(1)
				statsdClient.EXPECT().Count(sampler.MetricsRareMisses, int64(0), nil, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricsRareShrinks, float64(0), nil, gomock.Any()).Times(1)
			},
			expectStatsdWithFeature: func(statsdClient *mockStatsd.MockClientInterface) {
				statsdClient.EXPECT().Count(sampler.MetricSamplerKept, int64(1), []string{"sampler:priority", "sampling_priority:manual_keep", "target_service:serv1"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(1), []string{"sampler:priority", "sampling_priority:manual_keep", "target_service:serv1"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:no_priority"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:error"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:priority"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Count(sampler.MetricsRareHits, int64(0), nil, gomock.Any()).Times(1)
				statsdClient.EXPECT().Count(sampler.MetricsRareMisses, int64(0), nil, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricsRareShrinks, float64(0), nil, gomock.Any()).Times(1)
			},
		},
		"autodrop-error-sampled": {
			trace:           genSpan("", sampler.PriorityAutoDrop, 1),
			keep:            true,
			keepWithFeature: true,
			expectStatsd: func(statsdClient *mockStatsd.MockClientInterface) {
				statsdClient.EXPECT().Count(sampler.MetricSamplerKept, int64(1), []string{"sampler:error", "target_service:serv1"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(1), []string{"sampler:error", "target_service:serv1"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:no_priority"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(1), []string{"sampler:error"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(1), []string{"sampler:priority"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Count(sampler.MetricsRareHits, int64(0), nil, gomock.Any()).Times(1)
				statsdClient.EXPECT().Count(sampler.MetricsRareMisses, int64(0), nil, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricsRareShrinks, float64(0), nil, gomock.Any()).Times(1)
			},
			expectStatsdWithFeature: func(statsdClient *mockStatsd.MockClientInterface) {
				statsdClient.EXPECT().Count(sampler.MetricSamplerKept, int64(1), []string{"sampler:error", "target_service:serv1"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(1), []string{"sampler:error", "target_service:serv1"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:no_priority"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(1), []string{"sampler:error"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(1), []string{"sampler:priority"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Count(sampler.MetricsRareHits, int64(0), nil, gomock.Any()).Times(1)
				statsdClient.EXPECT().Count(sampler.MetricsRareMisses, int64(0), nil, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricsRareShrinks, float64(0), nil, gomock.Any()).Times(1)
			},
		},
		"autodrop-not-sampled": {
			trace:           genSpan("", sampler.PriorityAutoDrop, 0),
			keep:            false,
			keepWithFeature: false,
			expectStatsd: func(statsdClient *mockStatsd.MockClientInterface) {
				statsdClient.EXPECT().Count(sampler.MetricSamplerKept, gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(1), []string{"sampler:priority", "sampling_priority:auto_drop", "target_service:serv1"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:no_priority"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:error"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(1), []string{"sampler:priority"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Count(sampler.MetricsRareHits, int64(0), nil, gomock.Any()).Times(1)
				statsdClient.EXPECT().Count(sampler.MetricsRareMisses, int64(0), nil, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricsRareShrinks, float64(0), nil, gomock.Any()).Times(1)
			},
			expectStatsdWithFeature: func(statsdClient *mockStatsd.MockClientInterface) {
				statsdClient.EXPECT().Count(sampler.MetricSamplerKept, gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(1), []string{"sampler:priority", "sampling_priority:auto_drop", "target_service:serv1"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:no_priority"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:error"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(1), []string{"sampler:priority"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Count(sampler.MetricsRareHits, int64(0), nil, gomock.Any()).Times(1)
				statsdClient.EXPECT().Count(sampler.MetricsRareMisses, int64(0), nil, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricsRareShrinks, float64(0), nil, gomock.Any()).Times(1)
			},
		},
		"autokeep-dm-sampled": {
			trace:           genSpan("-9", sampler.PriorityAutoKeep, 0),
			keep:            true,
			keepWithFeature: true,
			expectStatsd: func(statsdClient *mockStatsd.MockClientInterface) {
				statsdClient.EXPECT().Count(sampler.MetricSamplerKept, int64(1), []string{"sampler:probabilistic", "target_service:serv1"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(1), []string{"sampler:probabilistic", "target_service:serv1"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:no_priority"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:error"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(1), []string{"sampler:priority"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Count(sampler.MetricsRareHits, int64(0), nil, gomock.Any()).Times(1)
				statsdClient.EXPECT().Count(sampler.MetricsRareMisses, int64(0), nil, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricsRareShrinks, float64(0), nil, gomock.Any()).Times(1)
			},
			expectStatsdWithFeature: func(statsdClient *mockStatsd.MockClientInterface) {
				statsdClient.EXPECT().Count(sampler.MetricSamplerKept, int64(1), []string{"sampler:probabilistic", "target_service:serv1"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(1), []string{"sampler:probabilistic", "target_service:serv1"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:no_priority"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:error"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(1), []string{"sampler:priority"}, gomock.Any()).Times(1)
				statsdClient.EXPECT().Count(sampler.MetricsRareHits, int64(0), nil, gomock.Any()).Times(1)
				statsdClient.EXPECT().Count(sampler.MetricsRareMisses, int64(0), nil, gomock.Any()).Times(1)
				statsdClient.EXPECT().Gauge(sampler.MetricsRareShrinks, float64(0), nil, gomock.Any()).Times(1)
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			statsd := mockStatsd.NewMockClientInterface(ctrl)
			metrics := sampler.NewMetrics(statsd)
			a := &Agent{
				NoPrioritySampler: sampler.NewNoPrioritySampler(cfg),
				ErrorsSampler:     sampler.NewErrorsSampler(cfg),
				PrioritySampler:   sampler.NewPrioritySampler(cfg, &sampler.DynamicConfig{}),
				RareSampler:       sampler.NewRareSampler(config.New()),
				EventProcessor:    newEventProcessor(cfg, statsd),
				SamplerMetrics:    metrics,
				conf:              cfg,
			}
			a.SamplerMetrics.Add(a.NoPrioritySampler, a.ErrorsSampler, a.PrioritySampler, a.RareSampler)
			tt.expectStatsd(statsd)
			keep, _ := a.traceSampling(now, info.NewReceiverStats().GetTagStats(info.Tags{}), &tt.trace)
			metrics.Report()
			assert.Equal(t, tt.keep, keep)
			assert.Equal(t, !tt.keep, tt.trace.TraceChunk.DroppedTrace)
			cfg.Features["error_rare_sample_tracer_drop"] = struct{}{}
			defer delete(cfg.Features, "error_rare_sample_tracer_drop")
			tt.expectStatsdWithFeature(statsd)
			keep, _ = a.traceSampling(now, info.NewReceiverStats().GetTagStats(info.Tags{}), &tt.trace)
			metrics.Report()
			assert.Equal(t, tt.keepWithFeature, keep)
			assert.Equal(t, !tt.keepWithFeature, tt.trace.TraceChunk.DroppedTrace)
		})
	}
}

func TestSample(t *testing.T) {
	now := time.Now()
	cfg := &config.AgentConfig{TargetTPS: 5, ErrorTPS: 1000, Features: make(map[string]struct{})}
	genSpan := func(decisionMaker string, priority sampler.SamplingPriority, err int32, exceptionInSpanEvent bool) traceutil.ProcessedTrace {
		root := &pb.Span{
			Service:  "serv1",
			Start:    now.UnixNano(),
			Duration: (100 * time.Millisecond).Nanoseconds(),
			Metrics:  map[string]float64{"_top_level": 1},
			Error:    err, // If 1, the Error Sampler will keep the trace, if 0, it will not be sampled
			Meta:     map[string]string{},
		}
		if exceptionInSpanEvent {
			root.Meta["_dd.span_events.has_exception"] = "true" // the Error Sampler will keep the trace
		}
		chunk := testutil.TraceChunkWithSpan(root)
		if decisionMaker != "" {
			chunk.Tags["_dd.p.dm"] = decisionMaker
			chunk.GetSpans()[0].Meta["_dd.p.dm"] = decisionMaker
		}
		pt := traceutil.ProcessedTrace{TraceChunk: chunk, Root: root}
		pt.TraceChunk.Priority = int32(priority)
		return pt
	}
	statsd := &statsd.NoOpClient{}
	tests := map[string]struct {
		trace           traceutil.ProcessedTrace
		etsEnabled      bool
		keep            bool
		keepWithFeature bool
	}{
		"userdrop-error-manual-dm-unsampled": {
			trace:           genSpan("-4", sampler.PriorityUserDrop, 1, false),
			keep:            false,
			keepWithFeature: false,
		},
		"userkeep-error-no-dm-sampled": {
			trace:           genSpan("", sampler.PriorityUserKeep, 1, false),
			keep:            true,
			keepWithFeature: true,
		},
		"userkeep-error-agent-dm-sampled": {
			trace:           genSpan("-1", sampler.PriorityUserKeep, 1, false),
			keep:            true,
			keepWithFeature: true,
		},
		"autodrop-error-sampled": {
			trace:           genSpan("", sampler.PriorityAutoDrop, 1, false),
			keep:            true,
			keepWithFeature: true,
		},
		"autodrop-not-sampled": {
			trace:           genSpan("", sampler.PriorityAutoDrop, 0, false),
			keep:            false,
			keepWithFeature: false,
		},
		"ets-userdrop-error-manual-dm-unsampled": {
			trace:           genSpan("-4", sampler.PriorityUserDrop, 1, false),
			etsEnabled:      true,
			keep:            true,
			keepWithFeature: true,
		},
		"ets-userdrop-errorspanevent-manual-dm-unsampled": {
			trace:           genSpan("-4", sampler.PriorityUserDrop, 1, false),
			etsEnabled:      true,
			keep:            true,
			keepWithFeature: true,
		},
		"ets-userdrop-manual-dm-unsampled": {
			trace:           genSpan("-4", sampler.PriorityUserDrop, 0, false),
			etsEnabled:      true,
			keep:            false,
			keepWithFeature: false,
		},
		"ets-userkeep-error-no-dm-sampled": {
			trace:           genSpan("", sampler.PriorityUserKeep, 1, false),
			etsEnabled:      true,
			keep:            true,
			keepWithFeature: true,
		},
		"ets-userkeep-error-agent-dm-sampled": {
			trace:           genSpan("-1", sampler.PriorityUserKeep, 1, false),
			etsEnabled:      true,
			keep:            true,
			keepWithFeature: true,
		},
		"ets-autodrop-error-sampled": {
			trace:           genSpan("", sampler.PriorityAutoDrop, 1, false),
			etsEnabled:      true,
			keep:            true,
			keepWithFeature: true,
		},
		"ets-autodrop-errorspanevent-sampled": {
			trace:           genSpan("", sampler.PriorityAutoDrop, 0, true),
			etsEnabled:      true,
			keep:            true,
			keepWithFeature: true,
		},
		"ets-autodrop-not-sampled": {
			trace:           genSpan("", sampler.PriorityAutoDrop, 0, false),
			etsEnabled:      true,
			keep:            false,
			keepWithFeature: false,
		},
	}
	for name, tt := range tests {
		cfg.ErrorTrackingStandalone = tt.etsEnabled
		a := &Agent{
			NoPrioritySampler: sampler.NewNoPrioritySampler(cfg),
			ErrorsSampler:     sampler.NewErrorsSampler(cfg),
			PrioritySampler:   sampler.NewPrioritySampler(cfg, &sampler.DynamicConfig{}),
			RareSampler:       sampler.NewRareSampler(config.New()),
			EventProcessor:    newEventProcessor(cfg, statsd),
			SamplerMetrics:    sampler.NewMetrics(statsd),
			conf:              cfg,
		}
		t.Run(name, func(t *testing.T) {
			keep, _ := a.sample(now, info.NewReceiverStats().GetTagStats(info.Tags{}), &tt.trace)
			assert.Equal(t, tt.keep, keep)
			assert.Equal(t, !tt.keep, tt.trace.TraceChunk.DroppedTrace)
			cfg.Features["error_rare_sample_tracer_drop"] = struct{}{}
			defer delete(cfg.Features, "error_rare_sample_tracer_drop")
			keep, _ = a.sample(now, info.NewReceiverStats().GetTagStats(info.Tags{}), &tt.trace)
			assert.Equal(t, tt.keepWithFeature, keep)
			assert.Equal(t, !tt.keepWithFeature, tt.trace.TraceChunk.DroppedTrace)
		})
	}
}

func TestSampleManualUserDropNoAnalyticsEvents(t *testing.T) {
	// This test exists to confirm previous behavior where we did not extract nor tag analytics events on
	// user manual drop traces
	now := time.Now()
	cfg := &config.AgentConfig{TargetTPS: 5, ErrorTPS: 1000, Features: make(map[string]struct{}), MaxEPS: 1000}
	root := &pb.Span{
		Service:  "serv1",
		Start:    now.UnixNano(),
		Duration: (100 * time.Millisecond).Nanoseconds(),
		Metrics:  map[string]float64{"_top_level": 1, "_dd1.sr.eausr": 1},
		Error:    0, // If 1, the Error Sampler will keep the trace, if 0, it will not be sampled
		Meta:     map[string]string{},
	}
	pt := traceutil.ProcessedTrace{TraceChunk: testutil.TraceChunkWithSpan(root), Root: root}
	pt.TraceChunk.Priority = -1
	statsd := &statsd.NoOpClient{}
	a := &Agent{
		NoPrioritySampler: sampler.NewNoPrioritySampler(cfg),
		ErrorsSampler:     sampler.NewErrorsSampler(cfg),
		PrioritySampler:   sampler.NewPrioritySampler(cfg, &sampler.DynamicConfig{}),
		RareSampler:       sampler.NewRareSampler(config.New()),
		EventProcessor:    newEventProcessor(cfg, statsd),
		SamplerMetrics:    sampler.NewMetrics(statsd),
		conf:              cfg,
	}
	keep, _ := a.sample(now, info.NewReceiverStats().GetTagStats(info.Tags{}), &pt)
	assert.False(t, keep)
	assert.Empty(t, pt.Root.Metrics["_dd.analyzed"])
}

func TestPartialSamplingFree(t *testing.T) {
	cfg := &config.AgentConfig{RareSamplerEnabled: false, BucketInterval: 10 * time.Second}
	dynConf := sampler.NewDynamicConfig()
	in := make(chan *api.Payload, 1000)
	statsd := &statsd.NoOpClient{}
	agnt := &Agent{
		Concentrator:      &mockConcentrator{},
		Blacklister:       filters.NewBlacklister(cfg.Ignore["resource"]),
		Replacer:          filters.NewReplacer(cfg.ReplaceTags),
		NoPrioritySampler: sampler.NewNoPrioritySampler(cfg),
		ErrorsSampler:     sampler.NewErrorsSampler(cfg),
		PrioritySampler:   sampler.NewPrioritySampler(cfg, &sampler.DynamicConfig{}),
		EventProcessor:    newEventProcessor(cfg, statsd),
		RareSampler:       sampler.NewRareSampler(config.New()),
		SamplerMetrics:    sampler.NewMetrics(statsd),
		TraceWriter:       &mockTraceWriter{},
		conf:              cfg,
		Timing:            &timing.NoopReporter{},
	}
	agnt.Receiver = api.NewHTTPReceiver(cfg, dynConf, in, agnt, telemetry.NewNoopCollector(), statsd, &timing.NoopReporter{})
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

	agnt.Concentrator.(*mockConcentrator).Reset()
	// big chunk should be cleaned as unsampled and passed through stats
	runtime.GC()
	runtime.ReadMemStats(&m)
	assert.Less(t, m.HeapInuse, uint64(50*1e6))

	assert.Len(t, agnt.TraceWriter.(*mockTraceWriter).payloads, 1)
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
		processor := newEventProcessor(conf, &statsd.NoOpClient{})
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
		pt := &traceutil.ProcessedTrace{
			TraceChunk: chunk,
			Root:       root,
		}
		_, numEvents, _ := processor.Process(pt)
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
// this means we won't compesate the overhead of TestFilteredByTagsing by dropping traces

func BenchmarkAgentTraceProcessingWithWorstCaseFiltering(b *testing.B) {
	c := config.New()
	c.Endpoints[0].APIKey = "test"
	c.Ignore["resource"] = []string{"[0-9]{3}", "foobar", "aaaaa?aaaa", "[^123]+_baz"}

	runTraceProcessingBenchmark(b, c)
}

func runTraceProcessingBenchmark(b *testing.B, c *config.AgentConfig) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	wg := sync.WaitGroup{}
	defer wg.Wait()
	defer cancelFunc()

	ta := NewTestAgent(ctx, c, telemetry.NewNoopCollector())
	wg.Add(1)
	go func() {
		defer wg.Done()
		ta.Run()
	}()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		ta.Process(&api.Payload{
			TracerPayload: testutil.TracerPayloadWithChunk(testutil.RandomTraceChunk(10, 8)),
			Source:        info.NewReceiverStats().GetTagStats(info.Tags{}),
		})
	}
}

// Mimics behaviour of agent Process function
func formatTrace(t pb.Trace) pb.Trace {
	for _, span := range t {
		a := &Agent{obfuscatorConf: &obfuscate.Config{}, conf: config.New()}
		a.obfuscateSpan(span)
		a.Truncate(span)
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
	filepath.Walk(folder, func(path string, info os.FileInfo, _ error) error {
		ext := filepath.Ext(path)
		if ext != ".msgp" {
			return nil
		}
		b.Run(info.Name(), benchThroughput(path))
		return nil
	})
}

type noopTraceWriter struct {
	count int
}

func (n *noopTraceWriter) Run() {}

func (n *noopTraceWriter) Stop() {}

func (n *noopTraceWriter) WriteChunks(_ *writer.SampledChunks) {
	n.count++
}

func (n *noopTraceWriter) FlushSync() error { return nil }

func (n *noopTraceWriter) UpdateAPIKey(_, _ string) {}

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
		agnt := NewTestAgent(ctx, cfg, telemetry.NewNoopCollector())
		defer cancelFunc()

		// start the agent without the trace and stats writers; we will be draining
		// these channels ourselves in the benchmarks, plus we don't want the writers
		// resource usage to show up in the results.
		noopWriter := &noopTraceWriter{}
		agnt.TraceWriter = noopWriter
		go agnt.Run()

		// wait for receiver to start:
		for {
			resp, err := http.Get("http://localhost:8126/v0.4/traces")
			if err != nil {
				time.Sleep(time.Millisecond)
				continue
			}
			resp.Body.Close()
			if resp.StatusCode == 400 {
				break
			}
		}

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
				default:
					if noopWriter.count == count {
						// processed everything!
						break loop
					}
				case <-timeout:
					// taking too long...
					b.Fatalf("time out at %d/%d", got, count)
				}
			}
		}
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
	bts, err := io.ReadAll(in)
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
		name          string
		features      string
		withFargate   bool
		in            *pb.ClientStatsPayload
		lang          string
		tracerVersion string
		containerID   string
		obfVersion    string
		out           *pb.ClientStatsPayload
	}{
		{
			name:     "containerID feature enabled, no fargate",
			features: "enable_cid_stats",
			in: &pb.ClientStatsPayload{
				Hostname:        "tracer_hots",
				Env:             "tracer_env",
				Version:         "code_version",
				ProcessTags:     "binary_name:bin",
				ProcessTagsHash: 123456789,
				Stats: []*pb.ClientStatsBucket{
					{
						Start:    1,
						Duration: 2,
						Stats: []*pb.ClientGroupedStats{
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
			containerID:   "abc123",
			out: &pb.ClientStatsPayload{
				Hostname:        "tracer_hots",
				Env:             "tracer_env",
				Version:         "code_version",
				Lang:            "java",
				TracerVersion:   "v1",
				ContainerID:     "abc123",
				ProcessTags:     "binary_name:bin",
				ProcessTagsHash: 123456789,
				Stats: []*pb.ClientStatsBucket{
					{
						Start:    1,
						Duration: 2,
						Stats: []*pb.ClientGroupedStats{
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
		{
			name:       "pre-obfuscated",
			obfVersion: strconv.Itoa(obfuscate.Version + 1),
			in: &pb.ClientStatsPayload{
				Hostname: "tracer_hots",
				Env:      "tracer_env",
				Version:  "code_version",
				Stats: []*pb.ClientStatsBucket{
					{
						Start:    1,
						Duration: 2,
						Stats: []*pb.ClientGroupedStats{
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
			containerID:   "abc123",
			out: &pb.ClientStatsPayload{
				Hostname:      "tracer_hots",
				Env:           "tracer_env",
				Version:       "code_version",
				Lang:          "java",
				TracerVersion: "v1",
				ContainerID:   "abc123",
				Stats: []*pb.ClientStatsBucket{
					{
						Start:    1,
						Duration: 2,
						Stats: []*pb.ClientGroupedStats{
							{
								Service:        "redis_service",
								Name:           "name_2",
								Resource:       "SET k v",
								HTTPStatusCode: 200,
								Type:           "redis",
							},
						},
					},
				},
			},
		},
		{
			name:     "containerID feature disabled, no fargate",
			features: "disable_cid_stats",
			in: &pb.ClientStatsPayload{
				Hostname: "tracer_hots",
				Env:      "tracer_env",
				Version:  "code_version",
				Stats: []*pb.ClientStatsBucket{
					{
						Start:    1,
						Duration: 2,
						Stats: []*pb.ClientGroupedStats{
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
			containerID:   "abc123",
			out: &pb.ClientStatsPayload{
				Hostname:      "tracer_hots",
				Env:           "tracer_env",
				Version:       "code_version",
				Lang:          "java",
				TracerVersion: "v1",
				ContainerID:   "",
				Stats: []*pb.ClientStatsBucket{
					{
						Start:    1,
						Duration: 2,
						Stats: []*pb.ClientGroupedStats{
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
		{
			name:        "containerID feature not configured, with fargate",
			features:    "",
			withFargate: true,
			in: &pb.ClientStatsPayload{
				Hostname: "tracer_hots",
				Env:      "tracer_env",
				Version:  "code_version",
				Stats: []*pb.ClientStatsBucket{
					{
						Start:    1,
						Duration: 2,
						Stats: []*pb.ClientGroupedStats{
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
			containerID:   "abc123",
			out: &pb.ClientStatsPayload{
				Hostname:      "tracer_hots",
				Env:           "tracer_env",
				Version:       "code_version",
				Lang:          "java",
				TracerVersion: "v1",
				ContainerID:   "abc123",
				Stats: []*pb.ClientStatsBucket{
					{
						Start:    1,
						Duration: 2,
						Stats: []*pb.ClientGroupedStats{
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
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			cfg := config.New()
			cfg.DefaultEnv = "agent_env"
			cfg.Hostname = "agent_hostname"
			cfg.MaxResourceLen = 5000
			cfg.Features[testCase.features] = struct{}{}
			if testCase.withFargate {
				cfg.FargateOrchestrator = config.OrchestratorECS
			}

			a := Agent{
				Blacklister:    filters.NewBlacklister([]string{"blocked_resource"}),
				obfuscatorConf: &obfuscate.Config{},
				Replacer:       filters.NewReplacer([]*config.ReplaceRule{{Name: "http.status_code", Pattern: "400", Re: regexp.MustCompile("400"), Repl: "200"}}),
				conf:           cfg,
			}

			out := a.processStats(testCase.in, testCase.lang, testCase.tracerVersion, testCase.containerID, testCase.obfVersion)
			assert.Equal(t, testCase.out, out)
		})
	}
}

func TestMergeDuplicates(t *testing.T) {
	in := &pb.ClientStatsBucket{
		Stats: []*pb.ClientGroupedStats{
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
	expected := &pb.ClientStatsBucket{
		Stats: []*pb.ClientGroupedStats{
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
	agnt := NewTestAgent(ctx, cfg, telemetry.NewNoopCollector())
	defer cancel()

	span := testutil.RandomSpan()
	pt := traceutil.ProcessedTrace{
		TraceChunk: testutil.TraceChunkWithSpan(span),
		Root:       span,
	}
	// before := traceutil.CopyTraceChunk(pt.TraceChunk)
	before := pt.TraceChunk.ShallowCopy()
	keep, numEvents := agnt.sample(time.Now(), info.NewReceiverStats().GetTagStats(info.Tags{}), &pt)
	assert.True(t, keep) // Score Sampler should keep the trace.
	assert.False(t, pt.TraceChunk.DroppedTrace)
	assert.Equal(t, before, pt.TraceChunk)
	assert.EqualValues(t, numEvents, 0)
}

// TestSpanSampling verifies that an incoming trace chunk that contains spans
// with "span sampling" tags results in those spans being sent to the trace
// writer in a sampled (kept) chunk. In other words, if a tracer marks spans as
// "span sampled," then the trace agent will not discard those spans.
//
// There are multiple cases to consider:
//
//  1. When the chunk would have been kept anyway (sampling priority > 0). In
//     this case, the tracer should not have added the span sampling tag.
//     Regardless, verify that the chunk is kept as expected.
//  2. When the chunk is dropped by the user, i.e. PriorityUserDrop sampling
//     priority. In this case, other samplers do not run at all, but the span
//     sampler still runs. The resulting trace chunk, if any, contains only
//     the spans that were tagged for span sampling, and the sampling priority
//     of the resulting chunk overall is PriorityUserKeep.
//     2a. Same as (2), except that only the local root span specifically is tagged
//     for span sampling. Verify that only the local root span is kept.
//     2b. Same as (2), except that only a non-local-root span specifically is
//     tagged for span sampling. Verify that only one span is kept.
//  3. When the chunk is dropped due to an agent-provided sample rate, i.e. with
//     PriorityAutoDrop priority. In this case, other samplers will run. Only if the
//     resulting decision is to drop the chunk, expect that the span sampler
//     will run and yield the same result as in case (2).
//  4. When the chunk is dropped due to an agent-provided sample rate, i.e. with
//     PriorityAutoDrop priority, but then the error sampler decides the keep
//     the chunk anyway. In this case, expect that the span sampler will _not_
//     run, and so the resulting chunk will contain all of its spans, even if
//     only some of them are tagged for span sampling.
func TestSpanSampling(t *testing.T) {
	// spanSamplingMetrics returns a map of numeric tags that contains the span
	// sampling metric (numeric tag) that tracers use to indicate that the span
	// should be kept by the span sampler.
	spanSamplingMetrics := func() map[string]float64 {
		metrics := make(map[string]float64, 1)
		// The value of this metric does not matter to the trace agent, but per
		// the single span ingestion control RFC it will be 8.
		metrics[sampler.KeySpanSamplingMechanism] = 8
		return metrics
	}

	tests := []struct {
		name    string
		payload *pb.TracerPayload
		// The payload is the input, the trace chunks are the output.
		checks func(*testing.T, *pb.TracerPayload, []*pb.TraceChunk)
	}{
		{
			name: "case 1: would have been kept anyway",
			payload: &pb.TracerPayload{
				Chunks: []*pb.TraceChunk{
					{
						Spans: []*pb.Span{
							{
								Service:  "testsvc",
								Name:     "parent",
								TraceID:  1,
								SpanID:   1,
								Start:    time.Now().Add(-time.Second).UnixNano(),
								Duration: time.Millisecond.Nanoseconds(),
							},
							{
								Service:  "testsvc",
								Name:     "child",
								TraceID:  1,
								SpanID:   2,
								Metrics:  spanSamplingMetrics(),
								Start:    time.Now().Add(-time.Second).UnixNano(),
								Duration: time.Millisecond.Nanoseconds(),
							},
						},
						// Span sampling metrics are included above, but it
						// doesn't matter because we're keeping this trace
						// anyway.
						Priority: int32(sampler.PriorityUserKeep),
					},
				},
			},
			checks: func(t *testing.T, payload *pb.TracerPayload, chunks []*pb.TraceChunk) {
				assert.Len(t, chunks, 1)
				chunk := chunks[0]
				assert.Equal(t, chunk.Priority, payload.Chunks[0].Priority)
				assert.Len(t, chunk.Spans, len(payload.Chunks[0].Spans))
				for i, after := range chunk.Spans {
					before := payload.Chunks[0].Spans[i]
					assert.Equal(t, before.TraceID, after.TraceID)
					assert.Equal(t, before.SpanID, after.SpanID)
					assert.Equal(t, before.Name, after.Name)
				}
			},
		},
		{
			name: "case 2a: keep one span: local root",
			payload: &pb.TracerPayload{
				Chunks: []*pb.TraceChunk{
					{
						Spans: []*pb.Span{
							{
								Service:  "testsvc",
								Name:     "parent",
								TraceID:  1,
								SpanID:   1,
								Metrics:  spanSamplingMetrics(),
								Start:    time.Now().Add(-time.Second).UnixNano(),
								Duration: time.Millisecond.Nanoseconds(),
							},
							{
								Service:  "testsvc",
								Name:     "child",
								TraceID:  1,
								SpanID:   2,
								ParentID: 1,
								Start:    time.Now().Add(-time.Second).UnixNano(),
								Duration: time.Millisecond.Nanoseconds(),
							},
						},
						// The tracer wants to drop the trace, but keep the parent span.
						Priority: int32(sampler.PriorityUserDrop),
					},
				},
			},
			checks: func(t *testing.T, _ *pb.TracerPayload, chunks []*pb.TraceChunk) {
				assert.Len(t, chunks, 1)
				chunk := chunks[0]
				// The span sampler kept the chunk.
				assert.False(t, chunk.DroppedTrace)
				assert.Equal(t, int32(sampler.PriorityUserKeep), chunk.Priority)
				// The span sampler discarded all but the sampled spans.
				assert.Len(t, chunk.Spans, 1)
				span := chunk.Spans[0]
				assert.Equal(t, uint64(1), span.TraceID)
				assert.Equal(t, uint64(1), span.SpanID)
				assert.Equal(t, "parent", span.Name)
			},
		},
		{
			name: "case 2b: keep one span: not local root",
			payload: &pb.TracerPayload{
				Chunks: []*pb.TraceChunk{
					{
						Spans: []*pb.Span{
							{
								Service:  "testsvc",
								Name:     "parent",
								TraceID:  1,
								SpanID:   1,
								Start:    time.Now().Add(-time.Second).UnixNano(),
								Duration: time.Millisecond.Nanoseconds(),
							},
							{
								Service:  "testsvc",
								Name:     "child",
								TraceID:  1,
								SpanID:   2,
								ParentID: 1,
								Metrics:  spanSamplingMetrics(),
								Start:    time.Now().Add(-time.Second).UnixNano(),
								Duration: time.Millisecond.Nanoseconds(),
							},
						},
						// The tracer wants to drop the trace, but keep the child span.
						Priority: int32(sampler.PriorityUserDrop),
					},
				},
			},
			checks: func(t *testing.T, _ *pb.TracerPayload, chunks []*pb.TraceChunk) {
				assert.Len(t, chunks, 1)
				chunk := chunks[0]
				// The span sampler kept the chunk.
				assert.False(t, chunk.DroppedTrace)
				assert.Equal(t, int32(sampler.PriorityUserKeep), chunk.Priority)
				// The span sampler discarded all but the sampled spans.
				assert.Len(t, chunk.Spans, 1)
				span := chunk.Spans[0]
				assert.Equal(t, uint64(1), span.TraceID)
				assert.Equal(t, uint64(2), span.SpanID)
				assert.Equal(t, "child", span.Name)
			},
		},
		{
			name: "case 3: keep spans from auto dropped trace",
			payload: &pb.TracerPayload{
				Chunks: []*pb.TraceChunk{
					{
						Spans: []*pb.Span{
							{
								Service:  "testsvc",
								Name:     "parent",
								TraceID:  1,
								SpanID:   1,
								Start:    time.Now().Add(-time.Second).UnixNano(),
								Duration: time.Millisecond.Nanoseconds(),
							},
							{
								Service:  "testsvc",
								Name:     "child",
								TraceID:  1,
								SpanID:   2,
								ParentID: 1,
								Metrics:  spanSamplingMetrics(),
								Start:    time.Now().Add(-time.Second).UnixNano(),
								Duration: time.Millisecond.Nanoseconds(),
							},
						},
						// The tracer wants to drop the trace, but keep the child span.
						Priority: int32(sampler.PriorityAutoDrop),
					},
				},
			},
			checks: func(t *testing.T, _ *pb.TracerPayload, chunks []*pb.TraceChunk) {
				assert.Len(t, chunks, 1)
				chunk := chunks[0]
				// The span sampler kept the chunk.
				assert.False(t, chunk.DroppedTrace)
				assert.Equal(t, int32(sampler.PriorityUserKeep), chunk.Priority)
				// The span sampler discarded all but the sampled spans.
				assert.Len(t, chunk.Spans, 1)
				span := chunk.Spans[0]
				assert.Equal(t, uint64(1), span.TraceID)
				assert.Equal(t, uint64(2), span.SpanID)
				assert.Equal(t, "child", span.Name)
			},
		},
		{
			name: "case 4: keep all spans from error sampler kept trace",
			payload: &pb.TracerPayload{
				Chunks: []*pb.TraceChunk{
					{
						Spans: []*pb.Span{
							{
								Service:  "testsvc",
								Name:     "parent",
								TraceID:  1,
								SpanID:   1,
								Start:    time.Now().Add(-time.Second).UnixNano(),
								Duration: time.Millisecond.Nanoseconds(),
							},
							{
								Service:  "testsvc",
								Name:     "child",
								TraceID:  1,
								SpanID:   2,
								ParentID: 1,
								Metrics:  spanSamplingMetrics(),
								// The error sampler will keep this trace because
								// this span has an error.
								Error:    1,
								Start:    time.Now().Add(-time.Second).UnixNano(),
								Duration: time.Millisecond.Nanoseconds(),
							},
						},
						// The tracer wants to drop the trace, but keep the child span.
						// However, there's an error in one of the spans ("child"), and
						// so all of the spans will be kept anyway.
						Priority: int32(sampler.PriorityAutoDrop),
					},
				},
			},
			checks: func(t *testing.T, payload *pb.TracerPayload, chunks []*pb.TraceChunk) {
				assert.Len(t, chunks, 1)
				chunk := chunks[0]
				// The error sampler kept the chunk.
				assert.False(t, chunk.DroppedTrace)
				// The span sampler did not discard any spans.
				assert.Len(t, chunk.Spans, len(payload.Chunks[0].Spans))
				for i, after := range chunk.Spans {
					before := payload.Chunks[0].Spans[i]
					assert.Equal(t, before.TraceID, after.TraceID)
					assert.Equal(t, before.SpanID, after.SpanID)
					assert.Equal(t, before.Name, after.Name)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.New()
			cfg.Endpoints[0].APIKey = "test"
			// Disable the rare sampler. Otherwise, it would consider every first
			// priority==0 chunk as rare. Instead, we use the error sampler to
			// cover the case where non-span samplers decide to keep a priority==0
			// chunk.
			cfg.RareSamplerEnabled = false
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			traceAgent := NewTestAgent(ctx, cfg, telemetry.NewNoopCollector())
			traceAgent.Process(&api.Payload{
				// The payload might get modified in-place, so first deep copy it so
				// that we have the original for comparison later.
				TracerPayload: proto.Clone(tc.payload).(*pb.TracerPayload),
				// a nil Source would trigger a panic
				Source: traceAgent.Receiver.Stats.GetTagStats(info.Tags{}),
			})
			assert.Len(t, traceAgent.TraceWriter.(*mockTraceWriter).payloads, 1)
			sampledChunks := traceAgent.TraceWriter.(*mockTraceWriter).payloads[0]
			tc.checks(t, tc.payload, sampledChunks.TracerPayload.Chunks)
			mco := traceAgent.Concentrator.(*mockConcentrator)
			require.Len(t, mco.stats, 1)
			stats := mco.stats[0]
			assert.Equal(t, len(tc.payload.Chunks[0].Spans), len(stats.Traces[0].TraceChunk.Spans))
		})
	}
}

func TestSingleSpanPlusAnalyticsEvents(t *testing.T) {
	cfg := config.New()
	cfg.Endpoints[0].APIKey = "test"
	// Disable the rare sampler. Otherwise, it would consider every first
	// priority==0 chunk as rare.
	cfg.RareSamplerEnabled = false
	cfg.TargetTPS = 0
	cfg.AnalyzedRateByServiceLegacy = map[string]float64{
		"testsvc": 1.0,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	traceAgent := NewTestAgent(ctx, cfg, telemetry.NewNoopCollector())
	singleSpanMetrics := map[string]float64{
		sampler.KeySpanSamplingMechanism:       8,
		sampler.KeySamplingRateEventExtraction: 1.0,
	}
	root := &pb.Span{
		Service:  "testsvc",
		Name:     "parent",
		TraceID:  1,
		SpanID:   1,
		Start:    time.Now().Add(-time.Second).UnixNano(),
		Duration: time.Millisecond.Nanoseconds(),
	}
	payload := &traceutil.ProcessedTrace{
		Root: root,
		TraceChunk: &pb.TraceChunk{
			Spans: []*pb.Span{
				root,
				{
					Service:  "testsvc",
					Name:     "child",
					TraceID:  1,
					SpanID:   2,
					ParentID: 1,
					Metrics:  singleSpanMetrics,
					Error:    0,
					Start:    time.Now().Add(-time.Second).UnixNano(),
					Duration: time.Millisecond.Nanoseconds(),
				},
			},
			Priority: int32(sampler.PriorityAutoDrop),
		},
	}
	var b bytes.Buffer
	oldLogger := log.SetLogger(log.NewBufferLogger(&b))
	defer func() { log.SetLogger(oldLogger) }()
	keep, numEvents := traceAgent.sample(time.Now(), info.NewReceiverStats().GetTagStats(info.Tags{}), payload)
	assert.Equal(t, "[WARN] Detected both analytics events AND single span sampling in the same trace. Single span sampling wins because App Analytics is deprecated.", b.String())
	assert.False(t, keep) //The sampling decision was FALSE but the trace itself is marked as not dropped
	assert.False(t, payload.TraceChunk.DroppedTrace)
	assert.Equal(t, 1, numEvents)
}

func TestSetFirstTraceTags(t *testing.T) {
	cfg := config.New()
	cfg.Endpoints[0].APIKey = "test"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	traceAgent := NewTestAgent(ctx, cfg, telemetry.NewNoopCollector())
	root := &pb.Span{
		Service:  "testsvc",
		Name:     "parent",
		TraceID:  1,
		SpanID:   1,
		Start:    time.Now().Add(-time.Second).UnixNano(),
		Duration: time.Millisecond.Nanoseconds(),
	}

	t.Run("NoConfigNoAction", func(t *testing.T) {
		traceAgent.setFirstTraceTags(root)
		_, ok := root.Meta[tagInstallID]
		assert.False(t, ok)
		_, ok = root.Meta[tagInstallType]
		assert.False(t, ok)
		_, ok = root.Meta[tagInstallTime]
		assert.False(t, ok)
	})

	cfg.InstallSignature.InstallID = "be7b577b-00d9-50a4-aa8d-345df57fd6f5"
	cfg.InstallSignature.InstallType = "manual"
	cfg.InstallSignature.InstallTime = time.Now().Unix()
	traceAgent = NewTestAgent(ctx, cfg, telemetry.NewNoopCollector())
	t.Run("SettingTagsFromInstallSignature", func(t *testing.T) {
		traceAgent.setFirstTraceTags(root)
		assert.Equal(t, cfg.InstallSignature.InstallID, root.Meta[tagInstallID])
		assert.Equal(t, cfg.InstallSignature.InstallType, root.Meta[tagInstallType])
		assert.Equal(t, fmt.Sprintf("%v", cfg.InstallSignature.InstallTime), root.Meta[tagInstallTime])

		// Also make sure the tags are only set once per agent instance,
		// calling setFirstTraceTags on another span by the same agent should have no effect
		anotherRoot := &pb.Span{
			Service:  "testsvc",
			Name:     "parent",
			TraceID:  1,
			SpanID:   1,
			Start:    time.Now().Add(-time.Second).UnixNano(),
			Duration: time.Millisecond.Nanoseconds(),
		}
		traceAgent.setFirstTraceTags(anotherRoot)
		_, ok := anotherRoot.Meta[tagInstallID]
		assert.False(t, ok)
		_, ok = anotherRoot.Meta[tagInstallType]
		assert.False(t, ok)
		_, ok = anotherRoot.Meta[tagInstallTime]
		assert.False(t, ok)

		// However, calling setFirstTraceTags on another span from a different service should set the tags again
		differentServiceRoot := &pb.Span{
			Service:  "discombobulator",
			Name:     "parent",
			TraceID:  2,
			SpanID:   2,
			Start:    time.Now().Add(-time.Second).UnixNano(),
			Duration: time.Millisecond.Nanoseconds(),
		}
		traceAgent.setFirstTraceTags(differentServiceRoot)
		assert.Equal(t, cfg.InstallSignature.InstallID, differentServiceRoot.Meta[tagInstallID])
		assert.Equal(t, cfg.InstallSignature.InstallType, differentServiceRoot.Meta[tagInstallType])
		assert.Equal(t, fmt.Sprintf("%v", cfg.InstallSignature.InstallTime), differentServiceRoot.Meta[tagInstallTime])
	})

	traceAgent = NewTestAgent(ctx, cfg, telemetry.NewNoopCollector())
	t.Run("NotClobberingExistingTags", func(t *testing.T) {
		timestamp := cfg.InstallSignature.InstallTime - 3600
		// Make the root span contain some tag values that conflict with what the agent has on record
		root = &pb.Span{
			Service:  "testsvc",
			Name:     "parent",
			TraceID:  1,
			SpanID:   1,
			Start:    time.Now().Add(-time.Second).UnixNano(),
			Duration: time.Millisecond.Nanoseconds(),
			Meta: map[string]string{
				tagInstallType: "k8s_single_step",
				tagInstallTime: strconv.FormatInt(timestamp, 10),
			},
		}

		traceAgent.setFirstTraceTags(root)
		assert.Equal(t, cfg.InstallSignature.InstallID, root.Meta[tagInstallID])
		assert.Equal(t, "k8s_single_step", root.Meta[tagInstallType])
		assert.Equal(t, strconv.FormatInt(timestamp, 10), root.Meta[tagInstallTime])
	})
}

func TestProcessedTrace(t *testing.T) {
	t.Run("all version tags set", func(t *testing.T) {
		root := &pb.Span{
			Service:  "testsvc",
			Name:     "parent",
			TraceID:  1,
			SpanID:   1,
			Start:    time.Now().Add(-time.Second).UnixNano(),
			Duration: time.Millisecond.Nanoseconds(),
			Meta:     map[string]string{"env": "test", "version": "v1.0.1"},
		}
		chunk := testutil.TraceChunkWithSpan(root)
		// Only fill out the relevant fields for processedTrace().
		apiPayload := &api.Payload{
			TracerPayload: &pb.TracerPayload{
				Env:         "test",
				Hostname:    "test-host",
				ContainerID: "1",
				Chunks:      []*pb.TraceChunk{chunk},
				AppVersion:  "v1.0.1",
			},
			ClientDroppedP0s: 1,
		}
		pt := processedTrace(apiPayload, chunk, root, "abc", "abc123")
		expectedPt := &traceutil.ProcessedTrace{
			TraceChunk:             chunk,
			Root:                   root,
			TracerEnv:              "test",
			TracerHostname:         "test-host",
			AppVersion:             "v1.0.1",
			GitCommitSha:           "abc123",
			ImageTag:               "abc",
			ClientDroppedP0sWeight: 1,
		}
		assert.Equal(t, expectedPt, pt)
	})

	t.Run("git commit sha from trace overrides container tag", func(t *testing.T) {
		root := &pb.Span{
			Service:  "testsvc",
			Name:     "parent",
			TraceID:  1,
			SpanID:   1,
			Start:    time.Now().Add(-time.Second).UnixNano(),
			Duration: time.Millisecond.Nanoseconds(),
			Meta:     map[string]string{"env": "test", "version": "v1.0.1", "_dd.git.commit.sha": "abc123"},
		}
		chunk := testutil.TraceChunkWithSpan(root)
		// Only fill out the relevant fields for processedTrace().
		apiPayload := &api.Payload{
			TracerPayload: &pb.TracerPayload{
				Env:         "test",
				Hostname:    "test-host",
				ContainerID: "1",
				Chunks:      []*pb.TraceChunk{chunk},
				AppVersion:  "v1.0.1",
			},
			ClientDroppedP0s: 1,
		}
		pt := processedTrace(apiPayload, chunk, root, "abc", "def456")
		expectedPt := &traceutil.ProcessedTrace{
			TraceChunk:             chunk,
			Root:                   root,
			TracerEnv:              "test",
			TracerHostname:         "test-host",
			AppVersion:             "v1.0.1",
			GitCommitSha:           "abc123",
			ImageTag:               "abc",
			ClientDroppedP0sWeight: 1,
		}
		assert.Equal(t, expectedPt, pt)
	})

	t.Run("no results from container lookup", func(t *testing.T) {
		root := &pb.Span{
			Service:  "testsvc",
			Name:     "parent",
			TraceID:  1,
			SpanID:   1,
			Start:    time.Now().Add(-time.Second).UnixNano(),
			Duration: time.Millisecond.Nanoseconds(),
			Meta:     map[string]string{"env": "test", "version": "v1.0.1", "_dd.git.commit.sha": "abc123"},
		}
		chunk := testutil.TraceChunkWithSpan(root)
		cfg := config.New()
		cfg.ContainerTags = func(_ string) ([]string, error) {
			return nil, nil
		}
		// Only fill out the relevant fields for processedTrace().
		apiPayload := &api.Payload{
			TracerPayload: &pb.TracerPayload{
				Env:         "test",
				Hostname:    "test-host",
				ContainerID: "1",
				Chunks:      []*pb.TraceChunk{chunk},
				AppVersion:  "v1.0.1",
			},
			ClientDroppedP0s: 1,
		}
		pt := processedTrace(apiPayload, chunk, root, "", "")
		expectedPt := &traceutil.ProcessedTrace{
			TraceChunk:             chunk,
			Root:                   root,
			TracerEnv:              "test",
			TracerHostname:         "test-host",
			AppVersion:             "v1.0.1",
			GitCommitSha:           "abc123",
			ImageTag:               "",
			ClientDroppedP0sWeight: 1,
		}
		assert.Equal(t, expectedPt, pt)
	})
}

func TestUpdateAPIKey(t *testing.T) {
	cfg := config.New()
	cfg.Endpoints[0].APIKey = "test"
	ctx, cancel := context.WithCancel(context.Background())
	agnt := NewTestAgent(ctx, cfg, telemetry.NewNoopCollector())
	defer cancel()

	agnt.UpdateAPIKey("test", "foo")
	tw := agnt.TraceWriter.(*mockTraceWriter)
	assert.Equal(t, "foo", tw.apiKey)
}
