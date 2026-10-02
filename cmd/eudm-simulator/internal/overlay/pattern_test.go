// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package overlay

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
)

func TestPatternProgressAndIndependentVariation(t *testing.T) {
	ctx := contextFixture("Google Chrome", "Google Chrome")
	ctx.Elapsed = 30 * time.Second
	for _, tc := range []struct {
		pattern schema.Pattern
		value   float64
	}{
		{steady(9), 9}, {ramp(10, 30), 20},
		{schema.Pattern{Step: &schema.StepPattern{Before: 5, After: 15, At: 50}}, 15},
		{schema.Pattern{Spike: &schema.SpikePattern{Baseline: 1, Peak: 11, At: 50, Duration: 10}}, 11},
	} {
		value, err := PatternValue(ctx, tc.pattern, "field")
		if err != nil {
			t.Fatal(err)
		}
		closeEnough(t, value, tc.value)
	}
	ctx.Group.BaselineVariance = .1
	first, err := PatternValue(ctx, steady(100), "field")
	if err != nil {
		t.Fatal(err)
	}
	for i := int64(0); i < 100; i++ {
		other := ctx
		other.SampleOrdinal = i
		_, _ = PatternValue(other, steady(10), "other")
	}
	second, err := PatternValue(ctx, steady(100), "field")
	if err != nil {
		t.Fatal(err)
	}
	if first != second || first < 90 || first > 110 {
		t.Fatal("variation depends on evaluation order or exceeds bound")
	}
	ctx.SampleOrdinal++
	third, err := PatternValue(ctx, steady(100), "field")
	if err != nil {
		t.Fatal(err)
	}
	if first == third {
		t.Fatal("sample ordinal does not participate in variation key")
	}
}
