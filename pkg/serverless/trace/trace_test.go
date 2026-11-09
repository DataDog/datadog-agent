// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package trace

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/cloudservice"
	serverlessInitTrace "github.com/DataDog/datadog-agent/cmd/serverless-init/trace"
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
	"github.com/DataDog/datadog-agent/pkg/trace/stats"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
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
	return &serverlessTraceAgent{ta: ta, convertToV1: true, cancel: cancel}, cfg
}

// capturingStatsWriter records the stats payloads written to it.
type capturingStatsWriter struct {
	payloads []*pb.StatsPayload
}

func (w *capturingStatsWriter) Write(p *pb.StatsPayload) { w.payloads = append(w.payloads, p) }

// statsBucketCount returns the number of stats buckets across payloads.
func statsBucketCount(payloads ...*pb.StatsPayload) int {
	n := 0
	for _, p := range payloads {
		for _, csp := range p.GetStats() {
			n += len(csp.GetStats())
		}
	}
	return n
}

// TestServerlessTraceAgentFlushAllForcesOpenStatsBuckets pins that FlushAll
// sends the concentrator's still-open bucket, which a regular flush holds back.
func TestServerlessTraceAgentFlushAllForcesOpenStatsBuckets(t *testing.T) {
	a, cfg := newTestServerlessTraceAgent(t)
	writer := &capturingStatsWriter{}
	concentrator := stats.NewConcentrator(cfg, writer, time.Now(), &statsd.NoOpClient{})
	a.ta.Concentrator = concentrator

	span := &pb.Span{
		Service: "svc", Name: "op", Resource: "res", TraceID: 1, SpanID: 1,
		Start: time.Now().UnixNano(), Duration: int64(time.Millisecond),
		Metrics: map[string]float64{"_top_level": 1},
	}
	concentrator.Add(stats.Input{Traces: []traceutil.ProcessedTrace{{
		TraceChunk: &pb.TraceChunk{Spans: []*pb.Span{span}},
		Root:       span,
	}}})
	require.Zero(t, statsBucketCount(concentrator.Flush(false)), "precondition: a regular flush holds back the open bucket")

	a.FlushAll()

	assert.Equal(t, 1, statsBucketCount(writer.payloads...), "FlushAll must force the open bucket into the stats writer")
}

// TestServerlessTraceAgentFlushAllSendsStatsSynchronously pins that the forced
// stats bucket has reached the intake by the time FlushAll returns, since the
// MicroVM can freeze right after.
func TestServerlessTraceAgentFlushAllSendsStatsSynchronously(t *testing.T) {
	var statsRequests atomic.Int32
	intake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		if r.URL.Path == "/api/v0.2/stats" {
			statsRequests.Add(1)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer intake.Close()

	cfg := config.New()
	cfg.Endpoints[0].APIKey = "test"
	cfg.Endpoints[0].Host = intake.URL
	cfg.SynchronousFlushing = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ta := traceagent.NewAgent(ctx, cfg, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, gzip.NewComponent())
	go ta.StatsWriter.Run()
	defer ta.StatsWriter.Stop()
	a := &serverlessTraceAgent{ta: ta, cancel: cancel}

	concentrator, ok := ta.Concentrator.(*stats.Concentrator)
	require.True(t, ok)
	span := &pb.Span{
		Service: "svc", Name: "op", Resource: "res", TraceID: 1, SpanID: 1,
		Start: time.Now().UnixNano(), Duration: int64(time.Millisecond),
		Metrics: map[string]float64{"_top_level": 1},
	}
	concentrator.Add(stats.Input{Traces: []traceutil.ProcessedTrace{{
		TraceChunk: &pb.TraceChunk{Spans: []*pb.Span{span}},
		Root:       span,
	}}})

	a.FlushAll()

	assert.Equal(t, int32(1), statsRequests.Load(), "the forced stats bucket must reach the intake before FlushAll returns")
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

// newJobSpan builds a span the way serverless-init builds the Cloud Run Jobs
// job span: a measured root span whose meta holds the trace tags, here with
// the error information of a failed job.
func newJobSpan() *pb.Span {
	span := serverlessInitTrace.InitSpan("my-job", "gcp.run.job.task", "my-job", "", time.Now().UnixNano(), map[string]string{
		"env":      "prod",
		"version":  "1.2.3",
		"job_name": "my-job",
	})
	span.Duration = int64(time.Second)
	span.Metrics = map[string]float64{"_dd.measured": 1}
	span.Error = 1
	span.Meta["error.msg"] = "task failed"
	span.Meta["exit_code"] = "1"
	return span
}

// TestServerlessTraceAgentProcessRoutesThroughV1 verifies that a job span
// submitted with SubmitSpan is converted to the v1 format and processed by
// the agent's v1 path (ProcessV1 / TraceWriterV1) rather than the legacy path,
// matching the default convert-traces behavior used for receiver-ingested
// traces.
func TestServerlessTraceAgentProcessRoutesThroughV1(t *testing.T) {
	sta, _ := newTestServerlessTraceAgent(t)
	legacyWriter := &mockTraceWriter{}
	v1Writer := &mockTraceWriterV1{}
	sta.ta.TraceWriter = legacyWriter
	sta.ta.TraceWriterV1 = v1Writer

	jobSpan := newJobSpan()
	serverlessInitTrace.SubmitSpan(jobSpan, cloudservice.CloudRunJobsOrigin, sta)

	assert.Empty(t, legacyWriter.payloads, "payload must not go through the legacy writer")
	require.Len(t, v1Writer.payloads, 1, "no payload was written through the v1 writer")
	got := v1Writer.payloads[0].TracerPayload
	require.Len(t, got.Chunks, 1)
	chunk := got.Chunks[0]
	assert.Equal(t, cloudservice.CloudRunJobsOrigin, chunk.Origin())
	assert.Equal(t, int32(sampler.PriorityUserKeep), chunk.Priority)
	assert.Equal(t, jobSpan.TraceID, chunk.LegacyTraceID())
	require.Len(t, chunk.Spans, 1)

	sp := chunk.Spans[0]
	assert.Equal(t, jobSpan.SpanID, sp.SpanID())
	assert.Equal(t, uint64(0), sp.ParentID())
	assert.Equal(t, "gcp.run.job.task", sp.Name())
	// Promoted tags are lifted to span fields.
	assert.Equal(t, "prod", sp.Env())
	assert.Equal(t, "1.2.3", sp.Version())
	assert.True(t, sp.Error())
	errMsg, ok := sp.GetAttributeAsString("error.msg")
	assert.True(t, ok)
	assert.Equal(t, "task failed", errMsg)
	exitCode, ok := sp.GetAttributeAsString("exit_code")
	assert.True(t, ok)
	assert.Equal(t, "1", exitCode)
	measured, ok := sp.GetAttributeAsFloat64("_dd.measured")
	assert.True(t, ok)
	assert.Equal(t, float64(1), measured)
	// The payload was generated in-process, not converted from a wire payload,
	// so it must not carry the conversion debug attribute.
	_, ok = sp.GetAttributeAsString("_dd.convertedv1")
	assert.False(t, ok, "in-process spans must not be tagged _dd.convertedv1")
}

// TestServerlessTraceAgentJobSpanAdoptsUserTraceIDV1 covers the Cloud Run Jobs
// flow on the default v1 path: a user root span is processed first, the job
// span modifier adopts its 128-bit trace ID and reparents it under the job
// span, and the job span submitted at job end carries that trace ID.
func TestServerlessTraceAgentJobSpanAdoptsUserTraceIDV1(t *testing.T) {
	sta, _ := newTestServerlessTraceAgent(t)
	v1Writer := &mockTraceWriterV1{}
	sta.ta.TraceWriterV1 = v1Writer

	jobSpan := newJobSpan()
	sta.SetSpanModifier(serverlessInitTrace.NewCloudRunJobsSpanModifier(jobSpan))

	const traceIDLow, traceIDHigh = uint64(0x1234), uint64(0x6a1b2c3d00000000)
	userSpan := &pb.Span{
		Service:  "my-job",
		Name:     "user.op",
		Resource: "user.op",
		TraceID:  traceIDLow,
		SpanID:   42,
		Start:    time.Now().UnixNano(),
		Duration: 1000,
		Meta:     map[string]string{"_dd.p.tid": "6a1b2c3d00000000"},
	}
	sta.Process(&api.Payload{
		Source: sta.ta.Receiver.Stats.GetTagStats(info.Tags{}),
		TracerPayload: testutil.TracerPayloadWithChunk(&pb.TraceChunk{
			Priority: int32(sampler.PriorityUserKeep),
			Spans:    []*pb.Span{userSpan},
		}),
	})
	serverlessInitTrace.SubmitSpan(jobSpan, cloudservice.CloudRunJobsOrigin, sta)

	require.Len(t, v1Writer.payloads, 2)
	userChunk := v1Writer.payloads[0].TracerPayload.Chunks[0]
	require.Len(t, userChunk.Spans, 1)
	assert.Equal(t, jobSpan.SpanID, userChunk.Spans[0].ParentID(), "user root span must be reparented under the job span")

	jobChunk := v1Writer.payloads[1].TracerPayload.Chunks[0]
	assert.Equal(t, traceIDLow, jobChunk.LegacyTraceID())
	assert.Equal(t, traceIDHigh, jobChunk.TraceIDHigh())
	require.Len(t, jobChunk.Spans, 1)
	assert.Equal(t, jobSpan.SpanID, jobChunk.Spans[0].SpanID())
}

// TestServerlessTraceAgentProcessDisableConvertTraces verifies that setting
// the disable-convert-traces feature flag makes the serverless trace agent
// fall back to the legacy processing path (Process / TraceWriter), as it does
// for receiver-ingested traces.
func TestServerlessTraceAgentProcessDisableConvertTraces(t *testing.T) {
	t.Setenv("DD_APM_FEATURES", "disable-convert-traces")
	setupTraceAgentTest(t)

	traceAgent := StartServerlessTraceAgent(StartServerlessTraceAgentArgs{
		Enabled:     true,
		LoadConfig:  &LoadConfig{Path: "./testdata/valid.yml", Tagger: noopimpl.NewComponent()},
		StopTimeout: 30 * time.Second,
	})
	defer traceAgent.Stop()
	require.IsType(t, &serverlessTraceAgent{}, traceAgent)
	sta := traceAgent.(*serverlessTraceAgent)
	require.False(t, sta.convertToV1)

	// Restore the real writers before the deferred Stop() runs, so they and
	// their goroutines are stopped rather than orphaned.
	legacyWriter := &mockTraceWriter{}
	v1Writer := &mockTraceWriterV1{}
	realWriter, realWriterV1 := sta.ta.TraceWriter, sta.ta.TraceWriterV1
	sta.ta.TraceWriter, sta.ta.TraceWriterV1 = legacyWriter, v1Writer
	defer func() { sta.ta.TraceWriter, sta.ta.TraceWriterV1 = realWriter, realWriterV1 }()

	jobSpan := newJobSpan()
	serverlessInitTrace.SubmitSpan(jobSpan, cloudservice.CloudRunJobsOrigin, sta)

	assert.Empty(t, v1Writer.payloads, "payload must not go through the v1 writer")
	require.Len(t, legacyWriter.payloads, 1, "no payload was written through the legacy writer")
	got := legacyWriter.payloads[0].TracerPayload
	require.Len(t, got.Chunks, 1)
	assert.Equal(t, cloudservice.CloudRunJobsOrigin, got.Chunks[0].Origin)
	require.Len(t, got.Chunks[0].Spans, 1)
	assert.Equal(t, jobSpan.SpanID, got.Chunks[0].Spans[0].SpanID)
}

// TestServerlessTraceAgentProcessAndFlushThroughV1 is an integration-style
// companion to TestServerlessTraceAgentProcessRoutesThroughV1: it starts a
// real serverless trace agent (synchronous flushing mode), replaces only its
// v1 trace writer with a mock, submits a job span via SubmitSpan, and verifies
// that a serverless Flush drains the v1 writer synchronously.
func TestServerlessTraceAgentProcessAndFlushThroughV1(t *testing.T) {
	setupTraceAgentTest(t)

	traceAgent := StartServerlessTraceAgent(StartServerlessTraceAgentArgs{
		Enabled: true,
		// The noop tagger keeps the ContainerTags closure (wired by the config
		// loader) from dereferencing a nil component when payload processing
		// resolves container tags.
		LoadConfig: &LoadConfig{Path: "./testdata/valid.yml", Tagger: noopimpl.NewComponent()},
		// Wait for the agent to fully stop before the test returns, so its
		// goroutines never leak into and race with other tests.
		StopTimeout: 30 * time.Second,
	})
	defer traceAgent.Stop()
	require.IsType(t, &serverlessTraceAgent{}, traceAgent)
	sta := traceAgent.(*serverlessTraceAgent)
	require.True(t, sta.convertToV1)

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

	jobSpan := newJobSpan()
	serverlessInitTrace.SubmitSpan(jobSpan, cloudservice.CloudRunJobsOrigin, sta)

	require.Len(t, mockWriter.payloads, 1, "no payload was written through the v1 writer")
	got := mockWriter.payloads[0].TracerPayload
	require.Len(t, got.Chunks, 1)
	require.Len(t, got.Chunks[0].Spans, 1)
	assert.Equal(t, jobSpan.SpanID, got.Chunks[0].Spans[0].SpanID())

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
