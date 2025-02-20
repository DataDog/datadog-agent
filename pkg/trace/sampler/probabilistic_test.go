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

	mockStatsd "github.com/DataDog/datadog-go/v5/statsd/mocks"
	"github.com/golang/mock/gomock"
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
			Features:                               map[string]struct{}{"probabilistic_sampler_full_trace_id": {}},
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
			Features:                               map[string]struct{}{"probabilistic_sampler_full_trace_id": {}},
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
			Features:                               map[string]struct{}{"probabilistic_sampler_full_trace_id": {}},
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
			Features:                               map[string]struct{}{"probabilistic_sampler_full_trace_id": {}},
		}
		sampler := NewProbabilisticSampler(conf, &statsd.NoOpClient{})
		sampled := sampler.Sample(&trace.Span{
			TraceID: 555,
			Meta:    map[string]string{"_dd.p.tid": hex.EncodeToString(tid[:8])},
		})
		assert.False(t, sampled)
	})
	t.Run("keep-dd-metrics", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		statsdClient := mockStatsd.NewMockClientInterface(ctrl)

		tid := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
		conf := &config.AgentConfig{
			ProbabilisticSamplerEnabled:            true,
			ProbabilisticSamplerHashSeed:           0,
			ProbabilisticSamplerSamplingPercentage: 41,
			Features:                               map[string]struct{}{"probabilistic_sampler_full_trace_id": {}},
		}
		sampler := NewProbabilisticSampler(conf, statsdClient)
		sampled := sampler.Sample(&trace.Span{
			TraceID: binary.BigEndian.Uint64(tid[8:]),
			Meta:    map[string]string{"_dd.p.tid": hex.EncodeToString(tid[:8])},
		})
		assert.True(t, sampled)

		statsdClient.EXPECT().Count(metricSamplerKept, int64(1), []string{"sampler:probabilistic"}, float64(1)).Times(1)
		statsdClient.EXPECT().Count(metricSamplerSeen, int64(1), []string{"sampler:probabilistic"}, float64(1)).Times(1)
		sampler.metrics.report()
	})
	t.Run("drop-dd-metrics", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		statsdClient := mockStatsd.NewMockClientInterface(ctrl)

		tid := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
		conf := &config.AgentConfig{
			ProbabilisticSamplerEnabled:            true,
			ProbabilisticSamplerHashSeed:           0,
			ProbabilisticSamplerSamplingPercentage: 40,
			Features:                               map[string]struct{}{"probabilistic_sampler_full_trace_id": {}},
		}
		sampler := NewProbabilisticSampler(conf, statsdClient)
		sampled := sampler.Sample(&trace.Span{
			TraceID: 555,
			Meta:    map[string]string{"_dd.p.tid": hex.EncodeToString(tid[:8])},
		})
		assert.False(t, sampled)

		statsdClient.EXPECT().Count(metricSamplerKept, gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
		statsdClient.EXPECT().Count(metricSamplerSeen, int64(1), []string{"sampler:probabilistic"}, float64(1)).Times(1)
		sampler.metrics.report()
	})
	t.Run("keep-dd-64-full", func(t *testing.T) {
		conf := &config.AgentConfig{
			ProbabilisticSamplerEnabled:            true,
			ProbabilisticSamplerHashSeed:           0,
			ProbabilisticSamplerSamplingPercentage: 40,
			Features:                               map[string]struct{}{"probabilistic_sampler_full_trace_id": {}},
		}
		sampler := NewProbabilisticSampler(conf, &statsd.NoOpClient{})
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
		sampler := NewProbabilisticSampler(conf, &statsd.NoOpClient{})
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
		sampler := NewProbabilisticSampler(conf, &statsd.NoOpClient{})
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
		sampler := NewProbabilisticSampler(conf, &statsd.NoOpClient{})
		sampled := sampler.Sample(&trace.Span{
			TraceID: 555,
			Meta:    map[string]string{"_dd.p.tid": hex.EncodeToString(tid[:8])},
		})
		assert.False(t, sampled)
	})
}

func TestSamplingRules(t *testing.T) {
	type spanTuple struct {
		sampled bool
		span    *trace.Span
	}
	tests := []struct {
		name        string
		agentConfig *config.AgentConfig
		spans       []spanTuple
	}{
		{
			name: "empty rule",
			agentConfig: &config.AgentConfig{
				ProbabilisticSamplerEnabled:            true,
				ProbabilisticSamplerSamplingPercentage: 100,
				ProbabilisticSamplerRules: []config.ProbabilisticSamplerRule{
					{},
				},
			},
			spans: []spanTuple{
				{sampled: true, span: &trace.Span{TraceID: 1, Service: "test-service", Name: "test-operation", Resource: "test-resource", Meta: map[string]string{"key1": "value1"}}},
				{sampled: true, span: &trace.Span{TraceID: 2, Service: "test-service-suffix", Name: "test-operation", Resource: "test-resource", Meta: map[string]string{"key1": "value1"}}},
				{sampled: true, span: &trace.Span{TraceID: 3, Service: "test-service", Name: "test-operation-suffix", Resource: "test-resource", Meta: map[string]string{"key1": "value1"}}},
				{sampled: true, span: &trace.Span{TraceID: 4, Service: "test-service", Name: "test-operation", Resource: "test-resource-suffix", Meta: map[string]string{"key1": "value1"}}},
				{sampled: true, span: &trace.Span{TraceID: 5, Service: "test-service", Name: "test-operation", Resource: "test-resource", Meta: map[string]string{"key1": "value1-suffix"}}},
				{sampled: true, span: &trace.Span{TraceID: 6, Service: "", Name: "test-operation", Resource: "test-resource", Meta: map[string]string{"key1": "value1"}}},
				{sampled: true, span: &trace.Span{TraceID: 7, Service: "test-service", Name: "", Resource: "test-resource", Meta: map[string]string{"key1": "value1"}}},
				{sampled: true, span: &trace.Span{TraceID: 8, Service: "test-service", Name: "test-operation", Resource: "", Meta: map[string]string{"key1": "value1"}}},
				{sampled: true, span: &trace.Span{TraceID: 9, Service: "test-service", Name: "test-operation", Resource: "test-resource", Meta: map[string]string{}}},
				{sampled: true, span: &trace.Span{TraceID: 10, Service: "test-service", Name: "test-operation", Resource: "test-resource", Meta: map[string]string{"key1": "value1", "key2": "value2"}}},
				{sampled: true, span: &trace.Span{TraceID: 11, Service: "", Name: "", Resource: "", Meta: map[string]string{}}},
			},
		},
		{
			name: "nil rule",
			agentConfig: &config.AgentConfig{
				ProbabilisticSamplerEnabled:            true,
				ProbabilisticSamplerSamplingPercentage: 100,
				ProbabilisticSamplerRules:              nil,
			},
			spans: []spanTuple{
				{sampled: true, span: &trace.Span{TraceID: 1, Service: "test-service", Name: "test-operation", Resource: "test-resource", Meta: map[string]string{"key1": "value1"}}},
				{sampled: true, span: &trace.Span{TraceID: 2, Service: "test-service-suffix", Name: "test-operation", Resource: "test-resource", Meta: map[string]string{"key1": "value1"}}},
				{sampled: true, span: &trace.Span{TraceID: 3, Service: "test-service", Name: "test-operation-suffix", Resource: "test-resource", Meta: map[string]string{"key1": "value1"}}},
				{sampled: true, span: &trace.Span{TraceID: 4, Service: "test-service", Name: "test-operation", Resource: "test-resource-suffix", Meta: map[string]string{"key1": "value1"}}},
				{sampled: true, span: &trace.Span{TraceID: 5, Service: "test-service", Name: "test-operation", Resource: "test-resource", Meta: map[string]string{"key1": "value1-suffix"}}},
				{sampled: true, span: &trace.Span{TraceID: 6, Service: "", Name: "test-operation", Resource: "test-resource", Meta: map[string]string{"key1": "value1"}}},
				{sampled: true, span: &trace.Span{TraceID: 7, Service: "test-service", Name: "", Resource: "test-resource", Meta: map[string]string{"key1": "value1"}}},
				{sampled: true, span: &trace.Span{TraceID: 8, Service: "test-service", Name: "test-operation", Resource: "", Meta: map[string]string{"key1": "value1"}}},
				{sampled: true, span: &trace.Span{TraceID: 9, Service: "test-service", Name: "test-operation", Resource: "test-resource", Meta: map[string]string{}}},
				{sampled: true, span: &trace.Span{TraceID: 10, Service: "test-service", Name: "test-operation", Resource: "test-resource", Meta: map[string]string{"key1": "value1", "key2": "value2"}}},
				{sampled: true, span: &trace.Span{TraceID: 11, Service: "", Name: "", Resource: "", Meta: map[string]string{}}},
			},
		},
		{
			name: "AND condition for each rule: a rule applies zero",
			agentConfig: &config.AgentConfig{
				ProbabilisticSamplerEnabled:            true,
				ProbabilisticSamplerSamplingPercentage: 100,
				ProbabilisticSamplerRules: []config.ProbabilisticSamplerRule{
					{
						Service:       "^test-service$",
						OperationName: "^test-operation$",
						ResourceName:  "^test-resource$",
						Attributes: map[string]string{
							"key1": "^value1$",
						},
						Percentage: 0,
					},
				},
			},
			spans: []spanTuple{
				{sampled: false, span: &trace.Span{TraceID: 1, Service: "test-service", Name: "test-operation", Resource: "test-resource", Meta: map[string]string{"key1": "value1"}}},
				{sampled: true, span: &trace.Span{TraceID: 2, Service: "test-service-suffix", Name: "test-operation", Resource: "test-resource", Meta: map[string]string{"key1": "value1"}}},
				{sampled: true, span: &trace.Span{TraceID: 3, Service: "test-service", Name: "test-operation-suffix", Resource: "test-resource", Meta: map[string]string{"key1": "value1"}}},
				{sampled: true, span: &trace.Span{TraceID: 4, Service: "test-service", Name: "test-operation", Resource: "test-resource-suffix", Meta: map[string]string{"key1": "value1"}}},
				{sampled: true, span: &trace.Span{TraceID: 5, Service: "test-service", Name: "test-operation", Resource: "test-resource", Meta: map[string]string{"key1": "value1-suffix"}}},
				{sampled: true, span: &trace.Span{TraceID: 6, Service: "", Name: "test-operation", Resource: "test-resource", Meta: map[string]string{"key1": "value1"}}},
				{sampled: true, span: &trace.Span{TraceID: 7, Service: "test-service", Name: "", Resource: "test-resource", Meta: map[string]string{"key1": "value1"}}},
				{sampled: true, span: &trace.Span{TraceID: 8, Service: "test-service", Name: "test-operation", Resource: "", Meta: map[string]string{"key1": "value1"}}},
				{sampled: true, span: &trace.Span{TraceID: 9, Service: "test-service", Name: "test-operation", Resource: "test-resource", Meta: map[string]string{}}},
				{sampled: false, span: &trace.Span{TraceID: 10, Service: "test-service", Name: "test-operation", Resource: "test-resource", Meta: map[string]string{"key1": "value1", "key2": "value2"}}},
				{sampled: true, span: &trace.Span{TraceID: 11, Service: "", Name: "", Resource: "", Meta: map[string]string{}}},
			},
		},
		{
			name: "AND condition for each rule: sampling percentage applies zero",
			agentConfig: &config.AgentConfig{
				ProbabilisticSamplerEnabled:            true,
				ProbabilisticSamplerSamplingPercentage: 0,
				ProbabilisticSamplerRules: []config.ProbabilisticSamplerRule{
					{
						Service:       "^test-service$",
						OperationName: "^test-operation$",
						ResourceName:  "^test-resource$",
						Attributes: map[string]string{
							"key1": "^value1$",
						},
						Percentage: 100,
					},
				},
			},
			spans: []spanTuple{
				{sampled: true, span: &trace.Span{TraceID: 1, Service: "test-service", Name: "test-operation", Resource: "test-resource", Meta: map[string]string{"key1": "value1"}}},
				{sampled: false, span: &trace.Span{TraceID: 2, Service: "test-service-suffix", Name: "test-operation", Resource: "test-resource", Meta: map[string]string{"key1": "value1"}}},
				{sampled: false, span: &trace.Span{TraceID: 3, Service: "test-service", Name: "test-operation-suffix", Resource: "test-resource", Meta: map[string]string{"key1": "value1"}}},
				{sampled: false, span: &trace.Span{TraceID: 4, Service: "test-service", Name: "test-operation", Resource: "test-resource-suffix", Meta: map[string]string{"key1": "value1"}}},
				{sampled: false, span: &trace.Span{TraceID: 5, Service: "test-service", Name: "test-operation", Resource: "test-resource", Meta: map[string]string{"key1": "value1-suffix"}}},
				{sampled: false, span: &trace.Span{TraceID: 6, Service: "", Name: "test-operation", Resource: "test-resource", Meta: map[string]string{"key1": "value1"}}},
				{sampled: false, span: &trace.Span{TraceID: 7, Service: "test-service", Name: "", Resource: "test-resource", Meta: map[string]string{"key1": "value1"}}},
				{sampled: false, span: &trace.Span{TraceID: 8, Service: "test-service", Name: "test-operation", Resource: "", Meta: map[string]string{"key1": "value1"}}},
				{sampled: false, span: &trace.Span{TraceID: 9, Service: "test-service", Name: "test-operation", Resource: "test-resource", Meta: map[string]string{}}},
				{sampled: true, span: &trace.Span{TraceID: 10, Service: "test-service", Name: "test-operation", Resource: "test-resource", Meta: map[string]string{"key1": "value1", "key2": "value2"}}},
				{sampled: false, span: &trace.Span{TraceID: 11, Service: "", Name: "", Resource: "", Meta: map[string]string{}}},
			},
		},
		{
			name: "OR condition between rules",
			agentConfig: &config.AgentConfig{
				ProbabilisticSamplerEnabled:            true,
				ProbabilisticSamplerSamplingPercentage: 0,
				ProbabilisticSamplerRules: []config.ProbabilisticSamplerRule{
					{
						OperationName: "^example1$",
						Percentage:    100,
					},
					{
						OperationName: "^example2$",
						Percentage:    100,
					},
					{
						OperationName: "^example3$",
						Percentage:    100,
					},
				},
			},
			spans: []spanTuple{
				{sampled: true, span: &trace.Span{TraceID: 1, Service: "test-service", Name: "example1", Resource: "test-resource", Meta: map[string]string{"key1": "value1"}}},
				{sampled: true, span: &trace.Span{TraceID: 2, Service: "", Name: "example1", Resource: "test-resource", Meta: map[string]string{"key1": "value1"}}},
				{sampled: true, span: &trace.Span{TraceID: 3, Service: "", Name: "example1", Resource: "", Meta: map[string]string{"key1": "value1"}}},
				{sampled: true, span: &trace.Span{TraceID: 4, Service: "", Name: "example1", Resource: "", Meta: map[string]string{}}},
				{sampled: true, span: &trace.Span{TraceID: 5, Service: "", Name: "example1", Resource: "", Meta: nil}},
				{sampled: true, span: &trace.Span{TraceID: 6, Service: "test-service", Name: "example2", Resource: "test-resource", Meta: map[string]string{"key1": "value1"}}},
				{sampled: true, span: &trace.Span{TraceID: 7, Service: "", Name: "example2", Resource: "test-resource", Meta: map[string]string{"key1": "value1"}}},
				{sampled: true, span: &trace.Span{TraceID: 8, Service: "", Name: "example2", Resource: "", Meta: map[string]string{"key1": "value1"}}},
				{sampled: true, span: &trace.Span{TraceID: 9, Service: "", Name: "example2", Resource: "", Meta: map[string]string{}}},
				{sampled: true, span: &trace.Span{TraceID: 10, Service: "", Name: "example2", Resource: "", Meta: nil}},
				{sampled: true, span: &trace.Span{TraceID: 11, Service: "test-service", Name: "example3", Resource: "test-resource", Meta: map[string]string{"key1": "value1"}}},
				{sampled: true, span: &trace.Span{TraceID: 12, Service: "", Name: "example3", Resource: "test-resource", Meta: map[string]string{"key1": "value1"}}},
				{sampled: true, span: &trace.Span{TraceID: 13, Service: "", Name: "example3", Resource: "", Meta: map[string]string{"key1": "value1"}}},
				{sampled: true, span: &trace.Span{TraceID: 14, Service: "", Name: "example3", Resource: "", Meta: map[string]string{}}},
				{sampled: true, span: &trace.Span{TraceID: 15, Service: "", Name: "example3", Resource: "", Meta: nil}},
				{sampled: false, span: &trace.Span{TraceID: 16, Service: "test-service", Name: "", Resource: "test-resource", Meta: map[string]string{"key1": "value1"}}},
				{sampled: false, span: &trace.Span{TraceID: 17, Service: "", Name: "", Resource: "test-resource", Meta: map[string]string{"key1": "value1"}}},
				{sampled: false, span: &trace.Span{TraceID: 18, Service: "", Name: "", Resource: "", Meta: map[string]string{"key1": "value1"}}},
				{sampled: false, span: &trace.Span{TraceID: 19, Service: "", Name: "", Resource: "", Meta: map[string]string{}}},
				{sampled: false, span: &trace.Span{TraceID: 20, Service: "", Name: "", Resource: "", Meta: nil}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sampler := NewProbabilisticSampler(tt.agentConfig, &statsd.NoOpClient{})
			for _, st := range tt.spans {
				sampled := sampler.Sample(st.span)
				assert.Equal(t, st.sampled, sampled, "mismatch: traceID: %d", st.span.TraceID)
			}
		})
	}
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
	cfg := processortest.NewNopSettings()
	pspCfg := &probabilisticsamplerprocessor.Config{
		SamplingPercentage: samplingPercent,
		HashSeed:           hashSeed,
	}

	conf := &config.AgentConfig{
		ProbabilisticSamplerEnabled:            true,
		ProbabilisticSamplerHashSeed:           hashSeed,
		ProbabilisticSamplerSamplingPercentage: samplingPercent,
		Features:                               map[string]struct{}{"probabilistic_sampler_full_trace_id": {}},
	}
	sampler := NewProbabilisticSampler(conf, &statsd.NoOpClient{})

	f.Add([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
	f.Fuzz(func(t *testing.T, tid []byte) {
		if len(tid) < 16 {
			t.Skip("need at least 16 bytes for trace id")
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
	t.Run("enabled", func(_ *testing.T) {
		conf := &config.AgentConfig{
			ProbabilisticSamplerEnabled:            true,
			ProbabilisticSamplerHashSeed:           22,
			ProbabilisticSamplerSamplingPercentage: 10,
		}
		sampler := NewProbabilisticSampler(conf, &statsd.NoOpClient{})
		sampler.Start()
		sampler.Stop()
	})
	t.Run("disabled", func(_ *testing.T) {
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
