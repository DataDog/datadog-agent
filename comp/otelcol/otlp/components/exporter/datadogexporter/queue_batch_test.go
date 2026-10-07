// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build test

package datadogexporter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/exporter/exportertest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	datadogconfig "github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/datadogconfig"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/serializerexporter"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/metricsclient"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/inframetadata"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/inframetadata/payload"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	pkgagent "github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/otel"

	gzip "github.com/DataDog/datadog-agent/comp/trace/compression/impl-gzip"
	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"
)

// batchingQueueConfig returns what the shipped default produces: the empty `batch:` key in
// cmd/otel-agent/dist/otel-config.yaml flips the Optional to Some(...), turning batching on.
func batchingQueueConfig() configoptional.Optional[exporterhelper.QueueBatchConfig] {
	q := exporterhelper.NewDefaultQueueConfig()
	q.Batch.GetOrInsertDefault()
	return configoptional.Some(q)
}

func TestQueueSettingsWithoutBatch(t *testing.T) {
	t.Run("clears batch and preserves everything else", func(t *testing.T) {
		in := batchingQueueConfig()
		in.Get().QueueSize = 4242
		in.Get().NumConsumers = 7
		require.True(t, in.Get().Batch.HasValue())

		out := queueSettingsWithoutBatch(in, zap.NewNop())

		require.True(t, out.HasValue(), "the queue itself must survive; only the batcher goes")
		assert.False(t, out.Get().Batch.HasValue())
		want := *in.Get()
		want.Batch = configoptional.None[exporterhelper.BatchConfig]()
		assert.Equal(t, want, *out.Get())

		// The same Config backs the metrics and logs exporters.
		assert.True(t, in.Get().Batch.HasValue(), "input config was mutated")
	})

	t.Run("no queue at all is passed through", func(t *testing.T) {
		in := configoptional.None[exporterhelper.QueueBatchConfig]()
		assert.Equal(t, in, queueSettingsWithoutBatch(in, zap.NewNop()))
	})

	t.Run("queue without batch is passed through", func(t *testing.T) {
		// What CreateDefaultConfig gives us without an explicit `batch:`.
		in := configoptional.Some(exporterhelper.NewDefaultQueueConfig())
		require.False(t, in.Get().Batch.HasValue())
		assert.Equal(t, in, queueSettingsWithoutBatch(in, zap.NewNop()))
	})
}

// TestExporterHelperBatchImpliesMutatesData pins the upstream premise: sending_queue.batch
// implies MutatesData. If a collector upgrade changes that, queueSettingsWithoutBatch can go.
func TestExporterHelperBatchImpliesMutatesData(t *testing.T) {
	ctx := context.Background()
	set := exportertest.NewNopSettings(Type)
	cfg := CreateDefaultConfig().(*datadogconfig.Config)
	cfg.QueueSettings = batchingQueueConfig()
	noop := func(context.Context, ptrace.Traces) error { return nil }

	withBatch, err := exporterhelper.NewTraces(ctx, set, cfg, noop,
		exporterhelper.WithQueue(cfg.QueueSettings))
	require.NoError(t, err)
	assert.True(t, withBatch.Capabilities().MutatesData,
		"sending_queue.batch is expected to force MutatesData upstream")

	withoutBatch, err := exporterhelper.NewTraces(ctx, set, cfg, noop,
		exporterhelper.WithQueue(queueSettingsWithoutBatch(cfg.QueueSettings, zap.NewNop())))
	require.NoError(t, err)
	assert.False(t, withoutBatch.Capabilities().MutatesData)
}

// TestTracesExporterIsNotMutating asserts the exporter the factory builds is read-only.
func TestTracesExporterIsNotMutating(t *testing.T) {
	cfg := CreateDefaultConfig().(*datadogconfig.Config)
	cfg.API.Key = "ddog_32_characters_long_api_key1"
	cfg.QueueSettings = batchingQueueConfig()
	require.True(t, cfg.QueueSettings.Get().Batch.HasValue())

	h := newTracesTestHarness(t)
	exp, err := h.factory.CreateTraces(context.Background(), h.params, cfg)
	require.NoError(t, err)

	assert.False(t, exp.Capabilities().MutatesData,
		"the traces exporter must stay read-only so the fanout consumer can share one copy")

	assert.True(t, cfg.QueueSettings.Get().Batch.HasValue(),
		"createTracesExporter must not write through to the shared config")
	assert.Equal(t, batchingQueueConfig(), cfg.QueueSettings)
}

// TestMetricsAndLogsExportersStillBatch: building the traces exporter first must not take
// batching away from the metrics and logs exporters built later from the same Config.
func TestMetricsAndLogsExportersStillBatch(t *testing.T) {
	cfg := CreateDefaultConfig().(*datadogconfig.Config)
	cfg.API.Key = "ddog_32_characters_long_api_key1"
	cfg.QueueSettings = batchingQueueConfig()

	h := newTracesTestHarness(t)
	_, err := h.factory.CreateTraces(context.Background(), h.params, cfg)
	require.NoError(t, err)

	set := exportertest.NewNopSettings(Type)
	noop := func(context.Context, ptrace.Traces) error { return nil }
	// Stand-in for the metrics/logs exporters, which are handed cfg.QueueSettings verbatim.
	batching, err := exporterhelper.NewTraces(context.Background(), set, cfg, noop,
		exporterhelper.WithQueue(cfg.QueueSettings))
	require.NoError(t, err)
	assert.True(t, batching.Capabilities().MutatesData,
		"metrics and logs must keep batching: fewer, larger payloads genuinely help there")
}

// Read-only safety: with the exporter non-mutating, the fanout consumer marks the shared
// ptrace.Traces read-only instead of cloning it, and pdata setters panic on read-only data.
// The tests below are the gate for that.

type tracesTestHarness struct {
	factory  exporter.Factory
	params   exporter.Settings
	agent    *pkgagent.Agent
	comp     testComponent
	reporter *inframetadata.Reporter
}

type nopPusher struct{}

func (nopPusher) Push(context.Context, payload.HostMetadata) error { return nil }

// newTracesTestHarness spins up a real trace agent against a throwaway intake.
func newTracesTestHarness(t *testing.T, tcfgOpts ...func(*config.AgentConfig)) *tracesTestHarness {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		rw.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(server.Close)

	tcfg := config.New()
	tcfg.ReceiverEnabled = false
	tcfg.TraceWriter.FlushPeriodSeconds = 0.1
	tcfg.Endpoints[0].APIKey = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	tcfg.Endpoints[0].Host = server.URL
	for _, opt := range tcfgOpts {
		opt(tcfg)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	agent := pkgagent.NewAgent(ctx, tcfg, telemetry.NewNoopCollector(), &ddgostatsd.NoOpClient{}, gzip.NewComponent())

	params := exportertest.NewNopSettings(Type)
	translator, err := attributes.NewTranslator(params.TelemetrySettings)
	require.NoError(t, err)
	comp := testComponent{agent, nil}
	comp.SetOTelAttributeTranslator(translator)

	reporter, err := inframetadata.NewReporter(zap.NewNop(), nopPusher{}, time.Hour)
	require.NoError(t, err)

	go agent.Run()

	return &tracesTestHarness{
		factory: NewFactory(comp, nil, nil, nil,
			metricsclient.NewStatsdClientWrapper(&ddgostatsd.NoOpClient{}),
			otel.NewDisabledGatewayUsage(), serializerexporter.TelemetryStore{}, nil),
		params:   params,
		agent:    agent,
		comp:     comp,
		reporter: reporter,
	}
}

// newExporter bypasses exporterhelper: ConsumeTraces is async once a queue is configured, so
// a panic would land on another goroutine. Calling consumeTraces inline keeps stacks useful.
func (h *tracesTestHarness) newExporter(cfg *datadogconfig.Config) *traceExporter {
	return newTracesExporter(context.Background(), h.params, cfg, h.comp,
		otel.NewDisabledGatewayUsage(), nil, nil, h.reporter)
}

func readOnlySafetyConfig() *datadogconfig.Config {
	cfg := CreateDefaultConfig().(*datadogconfig.Config)
	cfg.API.Key = "ddog_32_characters_long_api_key1"
	cfg.QueueSettings = batchingQueueConfig()
	// Exercise reporter.ConsumeResource on the read-only resource as well.
	cfg.HostMetadata.Enabled = true
	return cfg
}

// readOnlySafetyCorpus builds traces touching every part of the pdata tree the exporter walks.
func readOnlySafetyCorpus() ptrace.Traces {
	traces := ptrace.NewTraces()

	rspans := traces.ResourceSpans().AppendEmpty()
	res := rspans.Resource().Attributes()
	res.PutStr("datadog.host.name", "test-host")
	res.PutStr("service.name", "test-service")
	res.PutStr("deployment.environment", "test-env")
	res.PutStr("container.id", "0123456789abcdef")
	res.PutStr("k8s.pod.uid", "pod-uid")
	res.PutStr("k8s.namespace.name", "default")
	res.PutStr("aws.ecs.task.arn", "arn:aws:ecs:us-east-1:123456789012:task/cluster/task-id")
	res.PutStr("cloud.provider", "aws")
	rspans.Resource().SetDroppedAttributesCount(2)

	sspans := rspans.ScopeSpans().AppendEmpty()
	sspans.Scope().SetName("test-scope")
	sspans.Scope().SetVersion("v1.2.3")
	sspans.Scope().Attributes().PutStr("scope.attr", "scope-value")

	span := sspans.Spans().AppendEmpty()
	span.SetName("test-span")
	span.SetKind(ptrace.SpanKindServer)
	span.SetTraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 2, 3, 4})
	span.SetSpanID([8]byte{0, 0, 0, 0, 1, 2, 3, 4})
	span.TraceState().FromRaw("dd=s:1")
	span.SetDroppedAttributesCount(7)
	span.SetDroppedEventsCount(1)
	span.SetDroppedLinksCount(1)
	span.Status().SetCode(ptrace.StatusCodeError)
	span.Status().SetMessage("boom")
	span.Attributes().PutStr("http.method", "GET")
	span.Attributes().PutInt("http.status_code", 500)

	// One attribute of every value type MarshalEvents can meet.
	event := span.Events().AppendEmpty()
	event.SetName("exception")
	event.SetDroppedAttributesCount(5)
	attrs := event.Attributes()
	attrs.PutStr("str", "value")
	attrs.PutInt("int", 42)
	attrs.PutDouble("double", 4.2)
	attrs.PutBool("bool", true)
	attrs.PutEmptyBytes("bytes").FromRaw([]byte{1, 2, 3})
	attrs.PutEmpty("empty")
	attrs.PutEmptyMap("map").PutStr("nested", "value")
	sl := attrs.PutEmptySlice("slice")
	sl.AppendEmpty().SetStr("a")
	sl.AppendEmpty().SetInt(1)

	// The two other shapes MarshalEvents branches on.
	span.Events().AppendEmpty().SetName("bare")
	span.Events().AppendEmpty()

	link := span.Links().AppendEmpty()
	link.SetTraceID([16]byte{9, 9, 9, 9, 0, 0, 0, 0, 0, 0, 0, 0, 1, 2, 3, 4})
	link.SetSpanID([8]byte{9, 9, 9, 9, 1, 2, 3, 4})
	link.TraceState().FromRaw("dd=s:2")
	link.Attributes().PutStr("link.attr", "link-value")
	link.SetDroppedAttributesCount(3)

	// A second resource, so the per-resource loop runs more than once.
	other := traces.ResourceSpans().AppendEmpty()
	other.Resource().Attributes().PutStr("datadog.host.name", "other-host")
	otherSpan := other.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	otherSpan.SetName("other-span")
	otherSpan.SetTraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 5, 6, 7, 8})
	otherSpan.SetSpanID([8]byte{0, 0, 0, 0, 5, 6, 7, 8})

	return traces
}

func marshalTraces(t *testing.T, td ptrace.Traces) string {
	t.Helper()
	b, err := (&ptrace.JSONMarshaler{}).MarshalTraces(td)
	require.NoError(t, err)
	return string(b)
}

func TestConsumeTracesReadOnly(t *testing.T) {
	for _, receiveResourceSpansV2 := range []bool{true, false} {
		name := "ReceiveResourceSpansV2"
		if !receiveResourceSpansV2 {
			name = "ReceiveResourceSpansV1"
		}
		t.Run(name, func(t *testing.T) {
			h := newTracesTestHarness(t, func(tcfg *config.AgentConfig) {
				if !receiveResourceSpansV2 {
					tcfg.Features["disable_receive_resource_spans_v2"] = struct{}{}
				}
			})
			exp := h.newExporter(readOnlySafetyConfig())

			readOnly := readOnlySafetyCorpus()
			before := marshalTraces(t, readOnly)
			readOnly.MarkReadOnly()

			require.NotPanics(t, func() {
				require.NoError(t, exp.consumeTraces(context.Background(), readOnly))
			})
			assert.Equal(t, before, marshalTraces(t, readOnly),
				"the exporter must not modify the traces it is handed")
		})
	}
}

// TestConsumeTracesMutableAndReadOnlyAgree: same corpus, same trace-agent input, read-only or not.
func TestConsumeTracesMutableAndReadOnlyAgree(t *testing.T) {
	h := newTracesTestHarness(t)
	exp := h.newExporter(readOnlySafetyConfig())

	mutable := readOnlySafetyCorpus()
	require.NoError(t, exp.consumeTraces(context.Background(), mutable))

	readOnly := readOnlySafetyCorpus()
	readOnly.MarkReadOnly()
	require.NoError(t, exp.consumeTraces(context.Background(), readOnly))

	assert.Equal(t, marshalTraces(t, mutable), marshalTraces(t, readOnly))
}

// TestConsumeTracesReadOnlyConcurrent: the exporter and the connector now read the same
// ptrace.Traces concurrently instead of each getting a copy. Meaningful under -race.
func TestConsumeTracesReadOnlyConcurrent(t *testing.T) {
	h := newTracesTestHarness(t)
	exp := h.newExporter(readOnlySafetyConfig())

	shared := readOnlySafetyCorpus()
	before := marshalTraces(t, shared)
	shared.MarkReadOnly()

	const consumers = 8
	var wg sync.WaitGroup
	errs := make([]error, consumers)
	for i := range consumers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if i%2 == 0 {
				errs[i] = exp.consumeTraces(context.Background(), shared)
				return
			}
			// Stand in for the datadog connector.
			errs[i] = walkTraces(shared)
		}()
	}
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}
	assert.Equal(t, before, marshalTraces(t, shared))
}

// walkTraces reads every field the connector's translation touches, to pair with the
// exporter's reads under -race.
func walkTraces(td ptrace.Traces) error {
	rspans := td.ResourceSpans()
	for i := 0; i < rspans.Len(); i++ {
		rspan := rspans.At(i)
		rspan.Resource().Attributes().Range(func(string, pcommon.Value) bool { return true })
		for j := 0; j < rspan.ScopeSpans().Len(); j++ {
			sspan := rspan.ScopeSpans().At(j)
			for k := 0; k < sspan.Spans().Len(); k++ {
				span := sspan.Spans().At(k)
				span.Attributes().Range(func(string, pcommon.Value) bool { return true })
				for e := 0; e < span.Events().Len(); e++ {
					span.Events().At(e).Attributes().Range(func(string, pcommon.Value) bool { return true })
				}
				for l := 0; l < span.Links().Len(); l++ {
					span.Links().At(l).Attributes().Range(func(string, pcommon.Value) bool { return true })
				}
			}
		}
	}
	return nil
}
