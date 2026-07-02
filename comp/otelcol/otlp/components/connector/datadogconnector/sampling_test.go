// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package datadogconnector

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
			prob, ok := samplingProbFromTracestate(tt.traceState, nil)
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
		prob, ok := samplingProbFromTracestate("ot=th:zz", nil)
		assert.False(t, ok)
		assert.Equal(t, float64(0), prob)
	})
}
