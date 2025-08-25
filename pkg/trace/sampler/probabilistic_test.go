// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package sampler

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"testing"

	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/probabilisticsamplerprocessor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor/processortest"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
)

func TestProbabilisticSampler(t *testing.T) {
	t.Run("keep-otel", func(t *testing.T) {
		tid := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
		conf := &config.AgentConfig{
			ProbabilisticSamplerEnabled:            true,
			ProbabilisticSamplerHashSeed:           0,
			ProbabilisticSamplerSamplingPercentage: 41,
			Features:                               map[string]struct{}{"probabilistic_sampler_full_trace_id": {}},
		}
		sampler := NewProbabilisticSampler(conf)
		sampled := sampler.Sample(&trace.Span{
			TraceID: 555,
			Meta:    map[string]string{"otel.trace_id": hex.EncodeToString(tid)},
		})
		assert.True(t, sampled)
	})
	t.Run("drop-otel", func(t *testing.T) {
		tid := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
		conf := &config.AgentConfig{
			ProbabilisticSamplerEnabled:            true,
			ProbabilisticSamplerHashSeed:           0,
			ProbabilisticSamplerSamplingPercentage: 40,
			Features:                               map[string]struct{}{"probabilistic_sampler_full_trace_id": {}},
		}
		sampler := NewProbabilisticSampler(conf)
		sampled := sampler.Sample(&trace.Span{
			TraceID: 555,
			Meta:    map[string]string{"otel.trace_id": hex.EncodeToString(tid)},
		})
		assert.False(t, sampled)
	})
	t.Run("keep-dd", func(t *testing.T) {
		tid := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
		conf := &config.AgentConfig{
			ProbabilisticSamplerEnabled:            true,
			ProbabilisticSamplerHashSeed:           0,
			ProbabilisticSamplerSamplingPercentage: 41,
			Features:                               map[string]struct{}{"probabilistic_sampler_full_trace_id": {}},
		}
		sampler := NewProbabilisticSampler(conf)
		sampled := sampler.Sample(&trace.Span{
			TraceID: binary.BigEndian.Uint64(tid[8:]),
			Meta:    map[string]string{"_dd.p.tid": hex.EncodeToString(tid[:8])},
		})
		assert.True(t, sampled)
	})
	t.Run("drop-dd", func(t *testing.T) {
		tid := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
		conf := &config.AgentConfig{
			ProbabilisticSamplerEnabled:            true,
			ProbabilisticSamplerHashSeed:           0,
			ProbabilisticSamplerSamplingPercentage: 40,
			Features:                               map[string]struct{}{"probabilistic_sampler_full_trace_id": {}},
		}
		sampler := NewProbabilisticSampler(conf)
		sampled := sampler.Sample(&trace.Span{
			TraceID: 555,
			Meta:    map[string]string{"_dd.p.tid": hex.EncodeToString(tid[:8])},
		})
		assert.False(t, sampled)
	})
	t.Run("keep-dd-64-full", func(t *testing.T) {
		conf := &config.AgentConfig{
			ProbabilisticSamplerEnabled:            true,
			ProbabilisticSamplerHashSeed:           0,
			ProbabilisticSamplerSamplingPercentage: 40,
			Features:                               map[string]struct{}{"probabilistic_sampler_full_trace_id": {}},
		}
		sampler := NewProbabilisticSampler(conf)
		span := &trace.Span{
			TraceID: 555,
			Meta:    map[string]string{},
		}
		sampled := sampler.Sample(span)
		assert.True(t, sampled)
		assert.EqualValues(t, .4, span.Metrics["_dd.prob_sr"])
	})
	t.Run("drop-dd-64-full", func(t *testing.T) {
		conf := &config.AgentConfig{
			ProbabilisticSamplerEnabled:            true,
			ProbabilisticSamplerHashSeed:           0,
			ProbabilisticSamplerSamplingPercentage: 40,
			Features:                               map[string]struct{}{"probabilistic_sampler_full_trace_id": {}},
		}
		sampler := NewProbabilisticSampler(conf)
		sampled := sampler.Sample(&trace.Span{
			TraceID: 556,
			Meta:    map[string]string{},
		})
		assert.False(t, sampled)
	})
	t.Run("keep-dd-128", func(t *testing.T) {
		tid := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
		conf := &config.AgentConfig{
			ProbabilisticSamplerEnabled:            true,
			ProbabilisticSamplerHashSeed:           0,
			ProbabilisticSamplerSamplingPercentage: 70,
		}
		sampler := NewProbabilisticSampler(conf)
		sampled := sampler.Sample(&trace.Span{
			TraceID: binary.BigEndian.Uint64(tid[8:]),
			Meta:    map[string]string{"_dd.p.tid": hex.EncodeToString(tid[:8])},
		})
		assert.True(t, sampled)
	})
	t.Run("drop-dd-128", func(t *testing.T) {
		tid := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
		conf := &config.AgentConfig{
			ProbabilisticSamplerEnabled:            true,
			ProbabilisticSamplerHashSeed:           0,
			ProbabilisticSamplerSamplingPercentage: 68,
		}
		sampler := NewProbabilisticSampler(conf)
		sampled := sampler.Sample(&trace.Span{
			TraceID: 555,
			Meta:    map[string]string{"_dd.p.tid": hex.EncodeToString(tid[:8])},
		})
		assert.False(t, sampled)
	})
}

type mockConsumer struct {
	traces []ptrace.Traces
}

func (m *mockConsumer) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{
		MutatesData: false,
	}
}

func (m *mockConsumer) ConsumeTraces(_ context.Context, ts ptrace.Traces) error {
	m.traces = append(m.traces, ts)
	return nil
}

func FuzzConsistentWithOtel(f *testing.F) {
	hashSeed := uint32(555666)
	samplingPercent := float32(50)
	pspFactory := probabilisticsamplerprocessor.NewFactory()
	cfg := processortest.NewNopSettings(pspFactory.Type())
	pspCfg := &probabilisticsamplerprocessor.Config{
		SamplingPercentage: samplingPercent,
		HashSeed:           hashSeed,
		Mode:               "hash_seed",
	}

	conf := &config.AgentConfig{
		ProbabilisticSamplerEnabled:            true,
		ProbabilisticSamplerHashSeed:           hashSeed,
		ProbabilisticSamplerSamplingPercentage: samplingPercent,
		Features:                               map[string]struct{}{"probabilistic_sampler_full_trace_id": {}},
	}
	sampler := NewProbabilisticSampler(conf)

	f.Add([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
	f.Fuzz(func(t *testing.T, tid []byte) {
		if len(tid) < 16 {
			t.Skip("need at least 16 bytes for W3C Trace Context trace id")
		}
		// Skip zero trace IDs as they are invalid per W3C Trace Context
		// specification. OpenTelemetry follows the W3C Trace Context
		// specification for trace IDs. The test uses
		// opentelemetry-collector-contrib/processor/probabilisticsamplerprocessor
		// which handles trace IDs according to W3C standards.
		//
		// Per W3C Trace Context[1] "All bytes as zero
		// (00000000000000000000000000000000) is considered an invalid
		// value." The behavior of these two implementations with a
		// zero-value trace ID is undefined behavior.
		//
		// [1]: https://www.w3.org/TR/trace-context/#trace-id, section 3.2.2.3
		allZero := true
		for i := 0; i < 16 && i < len(tid); i++ {
			if tid[i] != 0 {
				allZero = false
				break
			}
		}
		if allZero {
			t.Skip("zero trace IDs are invalid per OpenTelemetry / W3C spec")
		}
		mc := &mockConsumer{} //Do this setup in here to avoid having to clear this data between tests
		tp, err := pspFactory.CreateTraces(context.Background(), cfg, pspCfg, mc)
		require.NoError(t, err)

		otelTrace := makeOtelTraceWithID(tid)
		err = tp.ConsumeTraces(context.Background(), otelTrace)
		assert.NoError(t, err)

		sampled := sampler.Sample(&trace.Span{
			TraceID: binary.BigEndian.Uint64(tid[8:]),
			Meta:    map[string]string{"_dd.p.tid": hex.EncodeToString(tid[:8])},
		})
		otelSampled := len(mc.traces) == 1
		if otelSampled != sampled {
			t.Logf("Trace ID: %x", tid)
			t.Logf("OTel sampled: %v, Datadog sampled: %v", otelSampled, sampled)
			t.Logf("Upper 8 bytes: %x, Lower 8 bytes: %x", tid[:8], tid[8:])
		}
		assert.Equal(t, otelSampled, sampled)
	})
}

func makeOtelTraceWithID(traceID []byte) ptrace.Traces {
	td := ptrace.NewTraces()
	tdResourceSpans := td.ResourceSpans()
	for i := 0; i < 1; i++ {
		rspan := tdResourceSpans.AppendEmpty()
		ilibspan := rspan.ScopeSpans().AppendEmpty()
		for s := 0; s < 3; s++ {
			span := ilibspan.Spans().AppendEmpty()
			span.SetTraceID(pcommon.TraceID(traceID))
		}
	}
	return td
}
