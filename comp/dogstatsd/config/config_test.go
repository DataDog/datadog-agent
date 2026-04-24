// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

// TestTruthTable exhaustively verifies the DogStatsD routing truth table from
// https://github.com/DataDog/saluki/issues/1334#issuecomment-4292253054.
func TestTruthTable(t *testing.T) {
	type tri struct {
		set bool
		val bool
	}
	cases := []struct {
		name          string
		useDogstatsd  tri
		dpEnabled     tri
		dpDogstatsd   tri
		wantEnabled   bool
		wantInternal  bool
		wantDataPlane bool
	}{
		{"true_true_true", tri{true, true}, tri{true, true}, tri{true, true}, true, false, true},
		{"true_true_false", tri{true, true}, tri{true, true}, tri{true, false}, true, true, false},
		{"true_true_null", tri{true, true}, tri{true, true}, tri{false, false}, true, false, true},
		{"true_false_null", tri{true, true}, tri{true, false}, tri{false, false}, true, true, false},
		{"true_null_true", tri{true, true}, tri{false, false}, tri{true, true}, true, true, false},
		{"true_null_false", tri{true, true}, tri{false, false}, tri{true, false}, true, true, false},
		{"true_null_null", tri{true, true}, tri{false, false}, tri{false, false}, true, true, false},
		{"false_true_true", tri{true, false}, tri{true, true}, tri{true, true}, false, false, false},
		{"false_true_false", tri{true, false}, tri{true, true}, tri{true, false}, false, false, false},
		{"false_true_null", tri{true, false}, tri{true, true}, tri{false, false}, false, false, false},
		{"false_false_null", tri{true, false}, tri{true, false}, tri{false, false}, false, false, false},
		{"false_null_true", tri{true, false}, tri{false, false}, tri{true, true}, false, false, false},
		{"false_null_false", tri{true, false}, tri{false, false}, tri{true, false}, false, false, false},
		{"false_null_null", tri{true, false}, tri{false, false}, tri{false, false}, false, false, false},
		{"null_true_true", tri{false, false}, tri{true, true}, tri{true, true}, true, false, true},
		{"null_true_false", tri{false, false}, tri{true, true}, tri{true, false}, true, true, false},
		{"null_true_null", tri{false, false}, tri{true, true}, tri{false, false}, true, false, true},
		{"null_false_null", tri{false, false}, tri{true, false}, tri{false, false}, true, true, false},
		{"null_null_true", tri{false, false}, tri{false, false}, tri{true, true}, true, true, false},
		{"null_null_false", tri{false, false}, tri{false, false}, tri{true, false}, true, true, false},
		{"null_null_null", tri{false, false}, tri{false, false}, tri{false, false}, true, true, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := configmock.New(t)
			if tc.useDogstatsd.set {
				cfg.SetWithoutSource("use_dogstatsd", tc.useDogstatsd.val)
			}
			if tc.dpEnabled.set {
				cfg.SetWithoutSource("data_plane.enabled", tc.dpEnabled.val)
			}
			if tc.dpDogstatsd.set {
				cfg.SetWithoutSource("data_plane.dogstatsd.enabled", tc.dpDogstatsd.val)
			}

			c := NewConfig(cfg)
			assert.Equal(t, tc.wantEnabled, c.Enabled(), "Enabled()")
			assert.Equal(t, tc.wantInternal, c.EnabledInternal(), "EnabledInternal()")
			assert.Equal(t, tc.wantDataPlane, c.enabledDataPlane(), "enabledDataPlane()")
		})
	}
}
