// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package transform

import (
	"math"
	"strconv"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/sampling"
	"go.opentelemetry.io/collector/pdata/pcommon"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
)

// keySamplingRateGlobal mirrors the unexported constant in
// github.com/DataDog/datadog-agent/pkg/trace/stats (weight.go).
// Must stay in sync: the Concentrator reads this key to compute span weight
// (weight = 1/_sample_rate).
const keySamplingRateGlobal = "_sample_rate"

// pValueNotSampled is the reserved p-value sentinel meaning "not sampled"
// in the consistent-probability sampling encoding.
const pValueNotSampled = 63

// samplingProbFromTracestate extracts the head-based sampling probability from a
// raw W3C tracestate string. Returns (probability, true) on success,
// (0, false) otherwise.
//
// Two encodings are supported:
//   - th (threshold): OTel collector-contrib pkg/sampling samplers.
//     e.g. "ot=th:8" → probability 0.5
//   - p (power-of-two): go.opentelemetry.io/contrib/samplers/probability/consistent.
//     e.g. "ot=p:1;r:1" → probability 2^-1 = 0.5
func samplingProbFromTracestate(raw string) (float64, bool) {
	if raw == "" {
		return 0, false
	}
	w3c, err := sampling.NewW3CTraceState(raw)
	if err != nil {
		// Malformed tracestate — log at debug so misconfigured tracers are
		// diagnosable without being noisy in healthy pipelines.
		log.Debugf("Failed to parse W3C tracestate %q for sampling probability: %v", raw, err)
		return 0, false
	}
	otel := w3c.OTelValue()

	// th encoding: used by the OTel collector-contrib pkg/sampling samplers.
	if th, ok := otel.TValueThreshold(); ok {
		return th.Probability(), true
	}

	// p encoding: used by go.opentelemetry.io/contrib/samplers/probability/consistent.
	// p:N means sampling probability = 2^-N (e.g. p:1 → 0.5, p:4 → 1/16).
	// Valid range is [0, 62]; p:63 is the reserved "not sampled" sentinel indicating
	// we should not sample the span.
	for _, kv := range otel.ExtraValues() {
		if kv.Key != "p" {
			continue
		}
		pVal, err := strconv.ParseUint(kv.Value, 10, 64)
		if err != nil {
			break
		}
		if pVal >= pValueNotSampled {
			// Sentinel value meaning "not sampled"; no probability to extract.
			break
		}
		if pVal == 0 {
			return 1.0, true
		}
		return math.Ldexp(1.0, -int(pVal)), true
	}

	return 0, false
}

// SetSampleRateFromTracestate decodes the head-based sampling probability from
// the raw W3C tracestate and sets it as _sample_rate on the span's Metrics —
// but only when a valid probability is decoded and _sample_rate is not already
// present (an explicit upstream value is preserved). This lets the APM stats
// Concentrator scale stats back up by the head-sampling weight (1/_sample_rate).
//
// The Concentrator reads weight only from the chunk root, so setting the value
// on non-root spans is harmless. Returns true if _sample_rate was set.
func SetSampleRateFromTracestate(span *pb.Span, rawTracestate string) bool {
	if span == nil {
		return false
	}
	if _, exists := span.Metrics[keySamplingRateGlobal]; exists {
		return false // preserve an explicitly set value from upstream
	}
	prob, ok := samplingProbFromTracestate(rawTracestate)
	if !ok {
		return false
	}
	if span.Metrics == nil {
		span.Metrics = make(map[string]float64)
	}
	span.Metrics[keySamplingRateGlobal] = prob
	return true
}

// SetSampleRateFromAttribute records an explicit _sample_rate set as a numeric
// span attribute by an upstream tracer onto the span's Metrics. This value takes
// precedence over the one decoded from the W3C tracestate: it is applied before
// SetSampleRateFromTracestate, whose "gated on absence" guard then preserves it.
// Only numeric (double/int) attributes are honored, mirroring how the full
// conversion maps span attributes to Metrics. Returns true if _sample_rate was set.
func SetSampleRateFromAttribute(span *pb.Span, sattr pcommon.Map) bool {
	if span == nil {
		return false
	}
	v, ok := sattr.Get(keySamplingRateGlobal)
	if !ok {
		return false
	}
	var rate float64
	switch v.Type() {
	case pcommon.ValueTypeDouble:
		rate = v.Double()
	case pcommon.ValueTypeInt:
		rate = float64(v.Int())
	default:
		return false
	}
	if span.Metrics == nil {
		span.Metrics = make(map[string]float64)
	}
	span.Metrics[keySamplingRateGlobal] = rate
	return true
}
