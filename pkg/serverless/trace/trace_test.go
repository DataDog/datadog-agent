// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package trace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/cloudservice"
	noopimpl "github.com/DataDog/datadog-agent/comp/core/tagger/impl-noop"
	gzip "github.com/DataDog/datadog-agent/comp/trace/compression/impl-gzip"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	traceagent "github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
	"github.com/DataDog/datadog-agent/pkg/trace/writer"

	"github.com/DataDog/datadog-go/v5/statsd"
)

// newV1SpanWithAttrs builds an idx.InternalSpan carrying the given string attributes.
func newV1SpanWithAttrs(attrs map[string]string) *idx.InternalSpan {
	strings := idx.NewStringTable()
	span := idx.NewInternalSpan(strings, &idx.Span{})
	for k, v := range attrs {
		span.SetStringAttribute(k, v)
	}
	return span
}

// newTestServerlessTraceAgent builds a minimal serverlessTraceAgent backed by
// a real *traceagent.Agent and spanModifier, without starting the agent's Run loop.
func newTestServerlessTraceAgent(t *testing.T) (*serverlessTraceAgent, *config.AgentConfig) {
	t.Helper()
	cfg := config.New()
	cfg.Endpoints[0].APIKey = "test"
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ta := traceagent.NewAgent(ctx, cfg, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, gzip.NewComponent())
	ta.SpanModifier = &spanModifier{ddOrigin: "lambda"}
	return &serverlessTraceAgent{ta: ta, cancel: cancel}, cfg
}

func setupTraceAgentTest(t *testing.T) {
	// ensure a free port is used for starting the trace agent
	port, err := testutil.FindTCPPort()
	require.NoError(t, err)
	t.Setenv("DD_RECEIVER_PORT", strconv.Itoa(port))
	configmock.New(t) // fresh config so BuildSchema picks it up
	require.Equal(t, port, pkgconfigsetup.Datadog().GetInt("apm_config.receiver_port"))
}

type LoadConfigMocked struct {
	Path string
}

type testSpanModifier struct {
	id int
}

func (*testSpanModifier) ModifySpan(*pb.TraceChunk, *pb.Span) {}

func (*testSpanModifier) ModifySpanV1(*idx.InternalTraceChunk, *idx.InternalSpan) {}

func TestSetSpanModifierReplacesBothProcessingPaths(t *testing.T) {
	oldModifier := &testSpanModifier{id: 1}
	newModifier := &testSpanModifier{id: 2}
	traceAgent := &serverlessTraceAgent{ta: &traceagent.Agent{
		SpanModifier:   oldModifier,
		SpanModifierV1: oldModifier,
	}}

	traceAgent.SetSpanModifier(newModifier)

	assert.Same(t, newModifier, traceAgent.ta.SpanModifier)
	assert.Same(t, newModifier, traceAgent.ta.SpanModifierV1)
	assert.Same(t, newModifier, traceAgent.GetSpanModifier())
}

func (l *LoadConfigMocked) Load() (*config.AgentConfig, error) {
	return nil, errors.New("error")
}

func TestStartEnabledTrueInvalidConfig(t *testing.T) {
	setupTraceAgentTest(t)

	agent := StartServerlessTraceAgent(StartServerlessTraceAgentArgs{
		Enabled:    true,
		LoadConfig: &LoadConfigMocked{},
	})
	defer agent.Stop()
	assert.NotNil(t, agent)
	assert.IsType(t, noopTraceAgent{}, agent)
}

func TestStartEnabledTrueValidConfigInvalidPath(t *testing.T) {
	setupTraceAgentTest(t)

	configmock.SetDefaultConfigType(t, "yaml")
	t.Setenv("DD_API_KEY", "x")
	agent := StartServerlessTraceAgent(StartServerlessTraceAgentArgs{
		Enabled:    true,
		LoadConfig: &LoadConfig{Path: "invalid.yml"},
	})
	defer agent.Stop()
	assert.NotNil(t, agent)
	assert.IsType(t, &serverlessTraceAgent{}, agent)
}

func TestStartEnabledTrueValidConfigValidPath(t *testing.T) {
	setupTraceAgentTest(t)

	agent := StartServerlessTraceAgent(StartServerlessTraceAgentArgs{
		Enabled:    true,
		LoadConfig: &LoadConfig{Path: "./testdata/valid.yml"},
	})
	defer agent.Stop()
	assert.NotNil(t, agent)
	assert.IsType(t, &serverlessTraceAgent{}, agent)
}

func TestFilterSpanFromRuntimeHttpSpan(t *testing.T) {
	httpSpanFromStatsD := pb.Span{
		Meta: map[string]string{
			"http.url": "http://127.0.0.1:8125/",
		},
	}
	assert.True(t, filterSpan(&httpSpanFromStatsD))
}

func TestFilterSpanFromRuntimeTcpSpan(t *testing.T) {
	tcpSpanFromStatsD := pb.Span{
		Meta: map[string]string{
			"tcp.remote.host": "127.0.0.1",
			"tcp.remote.port": "8125",
		},
	}
	assert.True(t, filterSpan(&tcpSpanFromStatsD))
}

func TestFilterSpanFromRuntimeDnsSpan(t *testing.T) {
	dnsSpanFromLocalhostAddress := pb.Span{
		Meta: map[string]string{
			"dns.address": "127.0.0.1",
		},
	}

	dnsSpanFromNonRoutableAddress := pb.Span{
		Meta: map[string]string{
			"dns.address": "0.0.0.0",
		},
	}

	assert.True(t, filterSpan(&dnsSpanFromLocalhostAddress))
	assert.True(t, filterSpan(&dnsSpanFromNonRoutableAddress))
}

func TestFilterSpanFromRuntimeLegitimateSpan(t *testing.T) {
	legitimateSpan := pb.Span{
		Meta: map[string]string{
			"http.url": "http://www.datadoghq.com",
		},
	}
	assert.False(t, filterSpan(&legitimateSpan))
}

func TestFilterSpanV1FromRuntimeHttpSpan(t *testing.T) {
	span := newV1SpanWithAttrs(map[string]string{
		"http.url": "http://127.0.0.1:8125/",
	})
	assert.True(t, filterSpanV1(span))
}

func TestFilterSpanV1FromRuntimeTcpSpan(t *testing.T) {
	span := newV1SpanWithAttrs(map[string]string{
		"tcp.remote.host": "127.0.0.1",
		"tcp.remote.port": "8125",
	})
	assert.True(t, filterSpanV1(span))
}

func TestFilterSpanV1FromRuntimeDnsSpan(t *testing.T) {
	localhost := newV1SpanWithAttrs(map[string]string{"dns.address": "127.0.0.1"})
	nonRoutable := newV1SpanWithAttrs(map[string]string{"dns.address": "0.0.0.0"})
	assert.True(t, filterSpanV1(localhost))
	assert.True(t, filterSpanV1(nonRoutable))
}

func TestFilterSpanV1FromRuntimeLegitimateSpan(t *testing.T) {
	span := newV1SpanWithAttrs(map[string]string{
		"http.url": "http://www.datadoghq.com",
	})
	assert.False(t, filterSpanV1(span))
}

func TestGetDDOriginCloudServices(t *testing.T) {
	serviceToEnvVar := map[string]string{
		"cloudrun":     cloudservice.ServiceNameEnvVar,
		"appservice":   cloudservice.WebsiteStack,
		"containerapp": cloudservice.ContainerAppNameEnvVar,
	}
	for service, envVar := range serviceToEnvVar {
		t.Setenv(envVar, "myService")
		assert.Equal(t, service, getDDOrigin())
		os.Unsetenv(envVar)
	}
}

func TestStartServerlessTraceAgentFunctionTags(t *testing.T) {
	const functionTagsPayloadTag = "_dd.tags.function"

	tests := []struct {
		name         string
		functionTags string
	}{
		{
			name:         "with function tags",
			functionTags: "env:production,service:my-service,version:1.0",
		},
		{
			name:         "with empty function tags",
			functionTags: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DD_RECEIVER_PORT", "0")
			t.Setenv("DD_APM_RECEIVER_SOCKET", filepath.Join(t.TempDir(), "apm.sock"))
			configmock.New(t)

			agent := StartServerlessTraceAgent(StartServerlessTraceAgentArgs{
				Enabled:      true,
				LoadConfig:   &LoadConfig{Path: "./testdata/valid.yml"},
				FunctionTags: tt.functionTags,
				// Wait for the agent to fully stop before the next subtest starts, so
				// its goroutines never leak into and race with the following case.
				StopTimeout: 30 * time.Second,
			})
			defer agent.Stop()

			assert.NotNil(t, agent)
			require.IsType(t, &serverlessTraceAgent{}, agent)

			// Access the underlying agent to check TracerPayloadModifier
			serverlessAgent := agent.(*serverlessTraceAgent)
			require.NotNil(t, serverlessAgent.ta.TracerPayloadModifier)

			payload := &pb.TracerPayload{}
			serverlessAgent.ta.TracerPayloadModifier.Modify(payload)
			if tt.functionTags == "" {
				assert.NotContains(t, payload.Tags, functionTagsPayloadTag)
			} else {
				require.NotNil(t, payload.Tags)
				assert.Equal(t, tt.functionTags, payload.Tags[functionTagsPayloadTag])
			}
		})
	}
}

func TestServerlessTraceAgentDisableTraceStats(t *testing.T) {
	tests := []struct {
		name       string
		envValue   string
		expectNoop bool
	}{
		{
			name:       "trace stats enabled by default",
			envValue:   "",
			expectNoop: false,
		},
		{
			name:       "trace stats disabled with true",
			envValue:   "true",
			expectNoop: true,
		},
		{
			name:       "trace stats enabled with false",
			envValue:   "false",
			expectNoop: false,
		},
		{
			name:       "trace stats enabled with other value",
			envValue:   "yes",
			expectNoop: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupTraceAgentTest(t)

			if tt.envValue != "" {
				t.Setenv(disableTraceStatsEnvVar, tt.envValue)
			}

			agent := StartServerlessTraceAgent(StartServerlessTraceAgentArgs{
				Enabled:    true,
				LoadConfig: &LoadConfig{Path: "./testdata/valid.yml"},
			})
			defer agent.Stop()

			assert.NotNil(t, agent)
			assert.IsType(t, &serverlessTraceAgent{}, agent)

			// Access the underlying agent to check concentrator type
			serverlessAgent := agent.(*serverlessTraceAgent)
			if tt.expectNoop {
				assert.IsType(t, &noopConcentrator{}, serverlessAgent.ta.Concentrator)
			} else {
				// Should not be noop concentrator
				assert.NotEqual(t, &noopConcentrator{}, serverlessAgent.ta.Concentrator)
			}
		})
	}
}

// mockTraceWriterV1 is a mock implementation of the agent.TraceWriterV1
// interface for testing
type mockTraceWriterV1 struct {
	mu       sync.Mutex
	payloads []*writer.SampledChunksV1
	flushes  int
}

func (m *mockTraceWriterV1) Stop() {}
func (m *mockTraceWriterV1) FlushSync() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.flushes++
	return nil
}
func (m *mockTraceWriterV1) UpdateAPIKey(_, _ string) {}

func (m *mockTraceWriterV1) WriteChunksV1(pkg *writer.SampledChunksV1) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.payloads = append(m.payloads, pkg)
}

// TestServerlessTraceAgentProcessRoutesThroughV1 verifies that
// ServerlessTraceAgent.Process submits payloads to the agent's v1 processing
// path (ProcessV1 / TraceWriterV1) rather than the legacy path, matching the
// default convert-traces behavior used for receiver-ingested traces.
func TestServerlessTraceAgentProcessRoutesThroughV1(t *testing.T) {
	sta, _ := newTestServerlessTraceAgent(t)
	mockWriter := &mockTraceWriterV1{}
	sta.ta.TraceWriterV1 = mockWriter

	// Build a payload the same way SubmitSpan does for inferred spans: a pb
	// tracer payload with a single kept chunk, converted to the v1 format.
	tc := testutil.RandomTraceChunk(2, 1)
	tc.Priority = int32(sampler.PriorityUserKeep)
	tp := testutil.TracerPayloadWithChunk(tc)

	sta.Process(&api.PayloadV1{
		Source:        sta.ta.Receiver.Stats.GetTagStats(info.Tags{}),
		TracerPayload: api.ConvertToIdx(tp, ""),
	})

	require.NotEmpty(t, mockWriter.payloads, "no payloads were written through the v1 writer")
	got := mockWriter.payloads[0].TracerPayload
	require.NotNil(t, got)
	require.NotEmpty(t, got.Chunks)
	require.NotEmpty(t, got.Chunks[0].Spans)
	// The submitted spans must be the converted v1 spans of the original chunk.
	assert.Equal(t, tc.Spans[0].SpanID, got.Chunks[0].Spans[0].SpanID())
	assert.Equal(t, len(tc.Spans), len(got.Chunks[0].Spans))
}

// TestServerlessTraceAgentProcessAndFlushThroughV1 is an integration-style
// companion to TestServerlessTraceAgentProcessRoutesThroughV1: it starts a
// real serverless trace agent (synchronous flushing mode), replaces only its
// v1 trace writer with a mock, submits a converted payload via Process (the
// path SubmitSpan uses for inferred spans), and verifies that a serverless
// Flush drains the v1 writer synchronously.
func TestServerlessTraceAgentProcessAndFlushThroughV1(t *testing.T) {
	setupTraceAgentTest(t)

	traceAgent := StartServerlessTraceAgent(StartServerlessTraceAgentArgs{
		Enabled: true,
		// The noop tagger keeps the ContainerTags closure (wired by the config
		// loader) from dereferencing a nil component when ProcessV1 resolves
		// container tags for the payload.
		LoadConfig: &LoadConfig{Path: "./testdata/valid.yml", Tagger: noopimpl.NewComponent()},
		// Wait for the agent to fully stop before the test returns, so its
		// goroutines never leak into and race with other tests.
		StopTimeout: 30 * time.Second,
	})
	defer traceAgent.Stop()
	require.IsType(t, &serverlessTraceAgent{}, traceAgent)
	sta := traceAgent.(*serverlessTraceAgent)

	// The agent is running in synchronous-flushing mode (set by
	// StartServerlessTraceAgent), so the v1 writer only sends on FlushSync;
	// the mock swap below makes that observable without network access.
	mockWriter := &mockTraceWriterV1{}
	// StartServerlessTraceAgent already started the real v1 writer (its
	// reporter and flush goroutines are running), so restore it before the
	// deferred Stop() runs — otherwise the original writer and its goroutines
	// are orphaned when Stop only stops the mock.
	realWriter := sta.ta.TraceWriterV1
	sta.ta.TraceWriterV1 = mockWriter
	defer func() { sta.ta.TraceWriterV1 = realWriter }()

	// Build a payload the same way SubmitSpan does for inferred spans.
	tc := testutil.RandomTraceChunk(2, 1)
	tc.Priority = int32(sampler.PriorityUserKeep)
	tp := testutil.TracerPayloadWithChunk(tc)

	sta.Process(&api.PayloadV1{
		Source:        sta.ta.Receiver.Stats.GetTagStats(info.Tags{}),
		TracerPayload: api.ConvertToIdx(tp, ""),
	})

	require.NotEmpty(t, mockWriter.payloads, "no payloads were written through the v1 writer")
	got := mockWriter.payloads[0].TracerPayload
	require.NotEmpty(t, got.Chunks)
	require.NotEmpty(t, got.Chunks[0].Spans)
	assert.Equal(t, tc.Spans[0].SpanID, got.Chunks[0].Spans[0].SpanID())
	assert.Equal(t, len(tc.Spans), len(got.Chunks[0].Spans))

	// The serverless Flush (invoked on invocation end / Stop by the
	// extension) must reach the v1 writer's synchronous flush.
	require.Equal(t, 0, mockWriter.flushes)
	sta.Flush()
	assert.Equal(t, 1, mockWriter.flushes, "serverless Flush must drain TraceWriterV1 via FlushSync")
}

// TestServerlessTraceAgentSetTagsUpdatesGlobalTagsAndSpanModifier verifies
// that the synchronous SetTags path (used once at startup, before the trace
// agent starts processing spans) still updates both GlobalTags and the span
// modifier, as it did before this fix.
func TestServerlessTraceAgentSetTagsUpdatesGlobalTagsAndSpanModifier(t *testing.T) {
	sta, cfg := newTestServerlessTraceAgent(t)

	sta.SetTags(map[string]string{"lambda_microvm_id": "vm-1"})

	assert.Equal(t, map[string]string{"lambda_microvm_id": "vm-1"}, cfg.GlobalTags)

	sm, ok := sta.ta.SpanModifier.(*spanModifier)
	require.True(t, ok)
	got := sm.tags.Load()
	require.NotNil(t, got)
	assert.Equal(t, map[string]string{"lambda_microvm_id": "vm-1"}, *got)
}

// TestServerlessTraceAgentUpdateRuntimeTagsDoesNotTouchGlobalTags is the core
// regression test for the fix: UpdateRuntimeTags (used by MicroVM's async
// /run hook) must only update the span modifier and must never write to
// GlobalTags, since GlobalTags is read unsynchronized by the trace agent's
// span-processing hot path.
func TestServerlessTraceAgentUpdateRuntimeTagsDoesNotTouchGlobalTags(t *testing.T) {
	sta, cfg := newTestServerlessTraceAgent(t)
	cfg.GlobalTags = map[string]string{"env": "prod"}

	sta.UpdateRuntimeTags(map[string]string{"lambda_microvm_id": "vm-2"})

	assert.Equal(t, map[string]string{"env": "prod"}, cfg.GlobalTags)

	sm, ok := sta.ta.SpanModifier.(*spanModifier)
	require.True(t, ok)
	got := sm.tags.Load()
	require.NotNil(t, got)
	assert.Equal(t, map[string]string{"lambda_microvm_id": "vm-2"}, *got)
}

// TestServerlessTraceAgentUpdateRuntimeTagsNoSpanModifier verifies
// UpdateRuntimeTags is safe to call when SpanModifier doesn't implement
// taggable (e.g. nil, or some other SpanModifier set via SetSpanModifier).
func TestServerlessTraceAgentUpdateRuntimeTagsNoSpanModifier(t *testing.T) {
	sta, _ := newTestServerlessTraceAgent(t)
	sta.ta.SpanModifier = nil

	assert.NotPanics(t, func() {
		sta.UpdateRuntimeTags(map[string]string{"lambda_microvm_id": "vm-3"})
	})
}
