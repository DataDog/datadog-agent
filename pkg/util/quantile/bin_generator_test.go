// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package quantile

import (
	"fmt"
	"testing"
)

func TestBinGenerator(t *testing.T) {
	generator := NewDDSketchBinGeneratorForAgent()
	if generator == nil {
		t.Fatal("Expected non-nil bin generator")
	}

	bounds := generator.GetBounds()
	if len(bounds) == 0 {
		t.Fatal("Expected non-empty bounds")
	}

	half := generator.Config.binLimit / 2
	for _, bound := range bounds {
		if bound.Key < Key(-half) || bound.Key >= Key(half) {
			t.Errorf("Key %d out of bounds", bound.Key)
		}
		if bound.Low < generator.Config.binLow(bound.Key) {
			t.Errorf("Low value %f for key %d is less than expected", bound.Low, bound.Key)
		}
	}
}

func TestBinGenerator_for_value(t *testing.T) {
	generator := NewDDSketchBinGeneratorForAgent()
	if generator == nil {
		t.Fatal("Expected non-nil bin generator")
	}

	testValues := []float64{2, 3.5, 4.2, 5.1, 6.7, 8.0, 10.0, -1.0, -2.5, -3.3, -4.8}
	for _, value := range testValues {
		key := generator.GetKeyForValue(value)
		bound, ok := generator.GetBound(key)
		if !ok {

			t.Errorf("Expected to find bound for key %d", key)
			fmt.Printf("Key for value %f: %d\n", value, key)
			continue
		}
		k := generator.GetKeyForValue(bound.Low)
		if k != key {
			t.Errorf("Expected key %d for value %f, got %d", key, value, k)
		}

	}
}
