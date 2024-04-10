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
	"github.com/DataDog/datadog-go/v5/statsd"
)

func TestProbabilisticSampler(t *testing.T) {
	t.Run("keep-otel", func(t *testing.T) {
		tid := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
		conf := &config.AgentConfig{
			ProbabilisticSamplerEnabled:            true,
			ProbabilisticSamplerHashSeed:           0,
			ProbabilisticSamplerSamplingPercentage: 41,
		}
		sampler := NewProbabilisticSampler(conf, &statsd.NoOpClient{})
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
		}
		sampler := NewProbabilisticSampler(conf, &statsd.NoOpClient{})
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
		}
		sampler := NewProbabilisticSampler(conf, &statsd.NoOpClient{})
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
		}
		sampler := NewProbabilisticSampler(conf, &statsd.NoOpClient{})
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
	cfg := processortest.NewNopCreateSettings()
	pspCfg := &probabilisticsamplerprocessor.Config{
		SamplingPercentage: samplingPercent,
		HashSeed:           hashSeed,
	}

	conf := &config.AgentConfig{
		ProbabilisticSamplerEnabled:            true,
		ProbabilisticSamplerHashSeed:           hashSeed,
		ProbabilisticSamplerSamplingPercentage: samplingPercent,
	}
	sampler := NewProbabilisticSampler(conf, &statsd.NoOpClient{})

	f.Add([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
	f.Fuzz(func(t *testing.T, tid []byte) {
		if len(tid) < 16 {
			t.Skip("need at least 16 bytes for trace id")
		}
		mc := &mockConsumer{} //Do this setup in here to avoid having to clear this data between tests
		tp, err := pspFactory.CreateTracesProcessor(context.Background(), cfg, pspCfg, mc)
		require.NoError(t, err)

		otelTrace := makeOtelTraceWithID(tid)
		err = tp.ConsumeTraces(context.Background(), otelTrace)
		assert.NoError(t, err)

		sampled := sampler.Sample(&trace.Span{
			TraceID: binary.BigEndian.Uint64(tid[8:]),
			Meta:    map[string]string{"_dd.p.tid": hex.EncodeToString(tid[:8])},
		})
		assert.Equal(t, len(mc.traces) == 1, sampled)
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

func TestProbabilisticSamplerStartStop(t *testing.T) {
	t.Run("enabled", func(t *testing.T) {
		conf := &config.AgentConfig{
			ProbabilisticSamplerEnabled:            true,
			ProbabilisticSamplerHashSeed:           22,
			ProbabilisticSamplerSamplingPercentage: 10,
		}
		sampler := NewProbabilisticSampler(conf, &statsd.NoOpClient{})
		sampler.Start()
		sampler.Stop()
	})
	t.Run("disabled", func(t *testing.T) {
		conf := &config.AgentConfig{
			ProbabilisticSamplerEnabled:            false,
			ProbabilisticSamplerHashSeed:           22,
			ProbabilisticSamplerSamplingPercentage: 10,
		}
		sampler := NewProbabilisticSampler(conf, &statsd.NoOpClient{})
		sampler.Start()
		sampler.Stop()
	})
}
