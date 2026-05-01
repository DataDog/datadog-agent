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

// ptr is a small helper that returns a pointer to the given value. Used below
// to express the tri-state (true, false, unset) of each config key in the
// truth table.
func ptr[T any](v T) *T { return &v }

// TestTruthTable exhaustively verifies the DogStatsD routing truth table from
// https://github.com/DataDog/saluki/issues/1334#issuecomment-4292253054.
//
// Each test row expresses the three inputs as *bool, where nil means "not
// set by the user".
func TestTruthTable(t *testing.T) {
	cases := []struct {
		name          string
		useDogstatsd  *bool
		dpEnabled     *bool
		dpDogstatsd   *bool
		wantEnabled   bool
		wantInternal  bool
		wantDataPlane bool
	}{
		{"true_true_true", ptr(true), ptr(true), ptr(true), true, false, true},
		{"true_true_false", ptr(true), ptr(true), ptr(false), true, true, false},
		{"true_true_null", ptr(true), ptr(true), nil, true, false, true},
		{"true_false_null", ptr(true), ptr(false), nil, true, true, false},
		{"true_null_true", ptr(true), nil, ptr(true), true, true, false},
		{"true_null_false", ptr(true), nil, ptr(false), true, true, false},
		{"true_null_null", ptr(true), nil, nil, true, true, false},
		{"false_true_true", ptr(false), ptr(true), ptr(true), false, false, false},
		{"false_true_false", ptr(false), ptr(true), ptr(false), false, false, false},
		{"false_true_null", ptr(false), ptr(true), nil, false, false, false},
		{"false_false_null", ptr(false), ptr(false), nil, false, false, false},
		{"false_null_true", ptr(false), nil, ptr(true), false, false, false},
		{"false_null_false", ptr(false), nil, ptr(false), false, false, false},
		{"false_null_null", ptr(false), nil, nil, false, false, false},
		{"null_true_true", nil, ptr(true), ptr(true), true, false, true},
		{"null_true_false", nil, ptr(true), ptr(false), true, true, false},
		{"null_true_null", nil, ptr(true), nil, true, false, true},
		{"null_false_null", nil, ptr(false), nil, true, true, false},
		{"null_null_true", nil, nil, ptr(true), true, true, false},
		{"null_null_false", nil, nil, ptr(false), true, true, false},
		{"null_null_null", nil, nil, nil, true, true, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := configmock.New(t)
			if tc.useDogstatsd != nil {
				cfg.SetWithoutSource("use_dogstatsd", *tc.useDogstatsd)
			}
			if tc.dpEnabled != nil {
				cfg.SetWithoutSource("data_plane.enabled", *tc.dpEnabled)
			}
			if tc.dpDogstatsd != nil {
				cfg.SetWithoutSource("data_plane.dogstatsd.enabled", *tc.dpDogstatsd)
			}

			c := NewConfig(cfg)
			assert.Equal(t, tc.wantEnabled, c.Enabled(), "Enabled()")
			assert.Equal(t, tc.wantInternal, c.EnabledInternal(), "EnabledInternal()")
			assert.Equal(t, tc.wantDataPlane, c.enabledDataPlane(), "enabledDataPlane()")
		})
	}
}
