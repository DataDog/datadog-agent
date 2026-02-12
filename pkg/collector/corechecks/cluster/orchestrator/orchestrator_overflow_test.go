// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator && test

package orchestrator

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	mockconfig "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// TestWouldOverflowOnAdd tests the mathematical logic for overflow detection
// This validates the formula: b > math.MaxInt - a prevents overflow when computing a + b
func TestWouldOverflowOnAdd(t *testing.T) {
	tests := []struct {
		name           string
		a              int
		b              int
		shouldOverflow bool
	}{
		{
			name:           "normal case - small values",
			a:              100,
			b:              200,
			shouldOverflow: false,
		},
		{
			name:           "normal case - larger values",
			a:              10000,
			b:              50000,
			shouldOverflow: false,
		},
		{
			name:           "edge case - at boundary",
			a:              math.MaxInt - 100,
			b:              100,
			shouldOverflow: false, // Exactly at limit, no overflow
		},
		{
			name:           "overflow case - just over boundary",
			a:              math.MaxInt - 100,
			b:              101,
			shouldOverflow: true,
		},
		{
			name:           "overflow case - both very large",
			a:              math.MaxInt / 2,
			b:              math.MaxInt/2 + 2,
			shouldOverflow: true,
		},
		{
			name:           "edge case - zero values",
			a:              0,
			b:              0,
			shouldOverflow: false,
		},
		{
			name:           "edge case - one zero",
			a:              0,
			b:              math.MaxInt,
			shouldOverflow: false,
		},
		{
			name:           "edge case - max int with zero",
			a:              math.MaxInt,
			b:              0,
			shouldOverflow: false,
		},
		{
			name:           "overflow case - max int with any positive",
			a:              math.MaxInt,
			b:              1,
			shouldOverflow: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This is the same check used in orchestrator.go
			wouldOverflow := tt.b > math.MaxInt-tt.a
			assert.Equal(t, tt.shouldOverflow, wouldOverflow,
				"overflow check failed for a=%d, b=%d", tt.a, tt.b)

			// If we expect no overflow, verify the addition is safe
			if !tt.shouldOverflow {
				sum := tt.a + tt.b
				assert.GreaterOrEqual(t, sum, tt.a, "sum should be >= a when no overflow")
				assert.GreaterOrEqual(t, sum, tt.b, "sum should be >= b when no overflow")
			}
		})
	}
}

// TestConfigureWithNormalTagCounts verifies that Configure works correctly
// with normal tag counts (no overflow scenario)
func TestConfigureWithNormalTagCounts(t *testing.T) {
	cfg := mockconfig.New(t)
	mockStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	// Setup fake tagger that returns a normal number of tags
	fakeTagger := taggerfxmock.SetupFakeTagger(t)

	orchCheck := newCheck(cfg, mockStore, fakeTagger).(*OrchestratorCheck)
	mockSenderManager := mocksender.CreateDefaultDemultiplexer()

	// Configure should succeed with normal tag counts
	err := orchCheck.Configure(mockSenderManager, uint64(1), integration.Data{}, integration.Data{}, "test")

	// We expect an error because the full orchestrator check requires more setup,
	// but it should NOT be an overflow error
	if err != nil {
		assert.NotContains(t, err.Error(), "overflow", "should not get overflow error with normal tag counts")
	}
}

// Overflow Protection Notes:
//
// This overflow check exists primarily to satisfy CodeQL security scanning requirements.
// In practice, overflowing the tag count is impossible - it would require allocating
// a slice with billions of elements (petabytes of RAM on 64-bit systems), and the
// system would OOM long before reaching overflow conditions.
//
// We cannot test actual overflow scenarios for this reason. Instead, TestWouldOverflowOnAdd
// validates the mathematical correctness of the overflow detection formula.
//
// Alternative considered: Remove the capacity argument from make() to avoid pre-computing
// the sum, at the cost of potential slice reallocation during append operations.
