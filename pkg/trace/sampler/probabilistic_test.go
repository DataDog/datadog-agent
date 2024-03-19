package sampler

import (
	"encoding/binary"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
)

func TestSample(t *testing.T) {
	t.Run("keep-otel", func(t *testing.T) {
		tid := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
		conf := &config.AgentConfig{
			ProbabilisticSamplerEnabled:            true,
			ProbabilisticSamplerHashSeed:           0,
			ProbabilisticSamplerSamplingPercentage: 41,
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
		}
		sampler := NewProbabilisticSampler(conf)
		sampled := sampler.Sample(&trace.Span{
			TraceID: 555,
			Meta:    map[string]string{"_dd.p.tid": hex.EncodeToString(tid)},
		})
		assert.False(t, sampled)
	})
}
