// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package transform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/pcommon"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
)

func TestSamplingProbFromTracestate(t *testing.T) {
	tests := []struct {
		name       string
		traceState string
		wantProb   float64
		wantOK     bool
	}{
		{
			name:       "50% sampling (th:8)",
			traceState: "ot=th:8",
			wantProb:   0.5,
			wantOK:     true,
		},
		{
			name:       "100% sampling (th:0)",
			traceState: "ot=th:0",
			wantProb:   1.0,
			wantOK:     true,
		},
		{
			name:       "p-encoding 50% (p:1)",
			traceState: "ot=p:1;r:1",
			wantProb:   0.5,
			wantOK:     true,
		},
		{
			name:       "p-encoding 6.25% (p:4)",
			traceState: "ot=p:4;r:4",
			wantProb:   0.0625,
			wantOK:     true,
		},
		{
			name:       "p-encoding 100% (p:0)",
			traceState: "ot=p:0",
			wantProb:   1.0,
			wantOK:     true,
		},
		{
			name:       "th absent rv present",
			traceState: "ot=rv:abcdefabcdefab",
			wantProb:   0,
			wantOK:     false,
		},
		{
			name:       "no tracestate",
			traceState: "",
			wantProb:   0,
			wantOK:     false,
		},
		{
			name:       "non-ot vendor field only",
			traceState: "zz=vendorcontent",
			wantProb:   0,
			wantOK:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prob, ok := samplingProbFromTracestate(tt.traceState)
			assert.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				assert.InDelta(t, tt.wantProb, prob, 1e-9)
			}
		})
	}
}

func TestSamplingProbFromTracestate_Malformed(t *testing.T) {
	// Malformed tracestate — must not panic and must return (0, false).
	// The value "ot=th:zz" parses the ot field but "zz" is not valid hex for
	// the th field, so NewW3CTraceState may return an error or silently skip;
	// either way ok=false.
	assert.NotPanics(t, func() {
		prob, ok := samplingProbFromTracestate("ot=th:zz")
		assert.False(t, ok)
		assert.Equal(t, float64(0), prob)
	})
}

func TestSetSampleRateFromTracestate(t *testing.T) {
	t.Run("sets rate when decodable and absent", func(t *testing.T) {
		s := &pb.Span{Metrics: map[string]float64{}}
		ok := SetSampleRateFromTracestate(s, "ot=th:8")
		assert.True(t, ok)
		assert.InDelta(t, 0.5, s.Metrics[keySamplingRateGlobal], 1e-9)
	})

	t.Run("preserves explicit upstream value", func(t *testing.T) {
		s := &pb.Span{Metrics: map[string]float64{keySamplingRateGlobal: 0.25}}
		ok := SetSampleRateFromTracestate(s, "ot=th:8")
		assert.False(t, ok)
		assert.InDelta(t, 0.25, s.Metrics[keySamplingRateGlobal], 1e-9)
	})

	t.Run("no-op when tracestate has no probability", func(t *testing.T) {
		s := &pb.Span{Metrics: map[string]float64{}}
		ok := SetSampleRateFromTracestate(s, "zz=vendor")
		assert.False(t, ok)
		_, exists := s.Metrics[keySamplingRateGlobal]
		assert.False(t, exists)
	})

	t.Run("nil Metrics map is allocated", func(t *testing.T) {
		s := &pb.Span{}
		ok := SetSampleRateFromTracestate(s, "ot=p:1;r:1")
		assert.True(t, ok)
		assert.InDelta(t, 0.5, s.Metrics[keySamplingRateGlobal], 1e-9)
	})

	t.Run("nil span is safe", func(t *testing.T) {
		assert.NotPanics(t, func() {
			assert.False(t, SetSampleRateFromTracestate(nil, "ot=th:8"))
		})
	})
}

func TestSetSampleRateFromAttribute(t *testing.T) {
	t.Run("sets rate from double attribute", func(t *testing.T) {
		s := &pb.Span{Metrics: map[string]float64{}}
		attrs := pcommon.NewMap()
		attrs.PutDouble(keySamplingRateGlobal, 0.25)
		ok := SetSampleRateFromAttribute(s, attrs)
		assert.True(t, ok)
		assert.InDelta(t, 0.25, s.Metrics[keySamplingRateGlobal], 1e-9)
	})

	t.Run("sets rate from int attribute", func(t *testing.T) {
		s := &pb.Span{Metrics: map[string]float64{}}
		attrs := pcommon.NewMap()
		attrs.PutInt(keySamplingRateGlobal, 1)
		ok := SetSampleRateFromAttribute(s, attrs)
		assert.True(t, ok)
		assert.InDelta(t, 1.0, s.Metrics[keySamplingRateGlobal], 1e-9)
	})

	t.Run("attribute takes precedence over tracestate", func(t *testing.T) {
		s := &pb.Span{Metrics: map[string]float64{}}
		attrs := pcommon.NewMap()
		attrs.PutDouble(keySamplingRateGlobal, 0.25)
		// Apply the attribute first, then the tracestate must not overwrite it.
		assert.True(t, SetSampleRateFromAttribute(s, attrs))
		assert.False(t, SetSampleRateFromTracestate(s, "ot=th:8"))
		assert.InDelta(t, 0.25, s.Metrics[keySamplingRateGlobal], 1e-9)
	})

	t.Run("no-op when attribute absent", func(t *testing.T) {
		s := &pb.Span{Metrics: map[string]float64{}}
		ok := SetSampleRateFromAttribute(s, pcommon.NewMap())
		assert.False(t, ok)
		_, exists := s.Metrics[keySamplingRateGlobal]
		assert.False(t, exists)
	})

	t.Run("no-op for non-numeric attribute", func(t *testing.T) {
		s := &pb.Span{Metrics: map[string]float64{}}
		attrs := pcommon.NewMap()
		attrs.PutStr(keySamplingRateGlobal, "0.25")
		ok := SetSampleRateFromAttribute(s, attrs)
		assert.False(t, ok)
		_, exists := s.Metrics[keySamplingRateGlobal]
		assert.False(t, exists)
	})

	t.Run("nil Metrics map is allocated", func(t *testing.T) {
		s := &pb.Span{}
		attrs := pcommon.NewMap()
		attrs.PutDouble(keySamplingRateGlobal, 0.5)
		ok := SetSampleRateFromAttribute(s, attrs)
		assert.True(t, ok)
		assert.InDelta(t, 0.5, s.Metrics[keySamplingRateGlobal], 1e-9)
	})

	t.Run("nil span is safe", func(t *testing.T) {
		assert.NotPanics(t, func() {
			assert.False(t, SetSampleRateFromAttribute(nil, pcommon.NewMap()))
		})
	})
}
